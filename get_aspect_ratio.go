package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

type VideoStream struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type FFProbeOutput struct {
	Streams []VideoStream `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe error: %w", err)
	}

	var ffprobeOutput FFProbeOutput
	if err := json.Unmarshal(out.Bytes(), &ffprobeOutput); err != nil {
		return "", fmt.Errorf("JSON unmarshal error: %w", err)
	}

	if len(ffprobeOutput.Streams) == 0 {
		return "", fmt.Errorf("no video streams found")
	}

	width, height := ffprobeOutput.Streams[0].Width, ffprobeOutput.Streams[0].Height
	if width == 0 || height == 0 {
		return "", fmt.Errorf("invalid dimensions")
	}

	ratio := float64(width) / float64(height)

	if ratio > 1.7 && ratio < 1.8 {
		return "16:9", nil
	} else if ratio > 0.55 && ratio < 0.58 {
		return "9:16", nil
	}
	return "other", nil
}
