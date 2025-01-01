package utils

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// GetVideoDuration gets the duration of the video in seconds.
func GetVideoDuration(filename string) (float64, error) {
	cmd := exec.Command("ffmpeg", "-i", "C:/personal/echo/server/temp/"+filename+".mp4")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	_ = cmd.Run() // Ignore the error

	// Parse the duration from the ffmpeg output
	re := regexp.MustCompile(`Duration: (\d+):(\d+):(\d+\.\d+)`)
	matches := re.FindStringSubmatch(errBuf.String())
	if matches == nil {
		return 0, fmt.Errorf("failed to parse video duration")
	}

	hours, _ := strconv.ParseFloat(matches[1], 64)
	minutes, _ := strconv.ParseFloat(matches[2], 64)
	seconds, _ := strconv.ParseFloat(matches[3], 64)
	duration := hours*3600 + minutes*60 + seconds
	return duration, nil
}

// ParseProgress parses the progress information from the ffmpeg output.
func ParseProgress(line string) float64 {
	re := regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return 0
	}

	hours, _ := strconv.ParseFloat(matches[1], 64)
	minutes, _ := strconv.ParseFloat(matches[2], 64)
	seconds, _ := strconv.ParseFloat(matches[3], 64)
	progress := hours*3600 + minutes*60 + seconds
	return progress
}
