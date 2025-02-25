package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	videoIDStr := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Could not find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Could not validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	if err = r.ParseMultipartForm(1 << 30); err != nil {
		respondWithError(w, http.StatusBadRequest, "Error parsing multipart form", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse video file", err)
		return
	}
	defer file.Close()

	ct, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if ct != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type. Only MP4 allowed.", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload-*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temporary file", err)
		return
	}
	defer tempFile.Close()

	if _, err = io.Copy(tempFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving temporary file", err)
		return
	}

	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error processing video for fast start", err)
		return
	}
	defer os.Remove(processedFilePath)

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed file", err)
		return
	}
	defer processedFile.Close()

	aspectRatio, err := getVideoAspectRatio(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error determining aspect ratio", err)
		return
	}

	prefix := "other"
	if aspectRatio == "16:9" {
		prefix = "landscape"
	} else if aspectRatio == "9:16" {
		prefix = "portrait"
	}

	randomBytes := make([]byte, 16)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error generating random file key", err)
		return
	}
	fileKey := fmt.Sprintf("%s/%s.mp4", prefix, hex.EncodeToString(randomBytes))

	putInput := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileKey,
		Body:        processedFile,
		ContentType: &ct,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), putInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file to S3", err)
		return
	}

	storedValue := fmt.Sprintf("%s,%s", cfg.s3Bucket, fileKey)
	video.VideoURL = aws.String(storedValue)

	if err = cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating video record", err)
		return
	}

	signedVideo, err := cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error generating presigned URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, signedVideo)
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error processing video for fast start: %w", err)
	}

	return outputFilePath, nil
}
