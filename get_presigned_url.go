package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s3Client)
	getObjectInput := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}
	psResult, err := presignClient.PresignGetObject(context.Background(), getObjectInput, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", fmt.Errorf("failed to presign get object: %w", err)
	}
	return psResult.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil || *video.VideoURL == "" {
		return video, nil
	}

	if strings.HasPrefix(*video.VideoURL, "http") {
		return video, nil
	}

	parts := strings.Split(*video.VideoURL, ",")
	if len(parts) != 2 {
		return video, fmt.Errorf("invalid video URL stored in database: %s", *video.VideoURL)
	}
	bucket := parts[0]
	key := parts[1]

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 15*time.Minute)
	if err != nil {
		return video, err
	}
	video.VideoURL = aws.String(presignedURL)
	return video, nil
}
