package transcoder

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// resolutionMap maps resolution names to their corresponding width and height values.
var resolutionMap = map[string]string{
	"1080p": "1920:1080",
	"720p":  "1280:720",
	"480p":  "852:480", // Changed from 854 to 852 to ensure even width
	"360p":  "640:360",
	"240p":  "426:240",
}

// standardResolutions defines available resolutions in ascending order
var standardResolutions = []string{
	"240p",
	"360p",
	"480p",
	"720p",
	"1080p",
}

type S3Handler struct {
	client                      *s3.Client
	downloadVideoBucket         string
	uploadTranscodedVideoBucket string
	uploader                    *manager.Uploader
	downloader                  *manager.Downloader
}

type Response struct {
	Resolutions []string
	Duration    int64
}

func NewS3Handler(downloadVideoBucket string, uploadTranscodedVideoBucket string) (*S3Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	return &S3Handler{
		client:                      client,
		downloadVideoBucket:         downloadVideoBucket,
		uploadTranscodedVideoBucket: uploadTranscodedVideoBucket,
		uploader:                    manager.NewUploader(client),
		downloader:                  manager.NewDownloader(client),
	}, nil
}

func (s *S3Handler) DownloadVideo(key string) (string, error) {
	// Create temp file to store the downloaded video
	tmpFile, err := os.CreateTemp("", "video-*.mp4")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	// Clean up the temp file if there is an error
	// defer is simply a way to register a function to be called later (after the surrounding function returns)
	defer tmpFile.Close()

	// Download the file from S3
	_, err = s.downloader.Download(context.Background(), tmpFile, &s3.GetObjectInput{
		Bucket: aws.String(s.downloadVideoBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}

	return tmpFile.Name(), nil
}

func (s *S3Handler) UploadTranscodedFile(localPath, s3Key string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Upload the file to S3
	_, err = s.uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(s.uploadTranscodedVideoBucket),
		Key:         aws.String(s3Key),
		Body:        file,
		ContentType: aws.String("application/x-mpegURL"),
	})

	return err
}

// parseResolution parses a resolution string in format "WxH" and returns width and height
func parseResolution(res string) (int, int, error) {
	var width, height int
	_, err := fmt.Sscanf(res, "%dx%d", &width, &height)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid resolution format: %s", res)
	}
	return width, height, nil
}

func getApplicableResolutions(originalWidth, originalHeight int) []string {
	var applicable []string

	// Determine original resolution category
	originalRes := ""
	if originalWidth >= 1920 && originalHeight >= 1080 {
		originalRes = "1080p"
	} else if originalWidth >= 1280 && originalHeight >= 720 {
		originalRes = "720p"
	} else if originalWidth >= 854 && originalHeight >= 480 {
		originalRes = "480p"
	} else if originalWidth >= 640 && originalHeight >= 360 {
		originalRes = "360p"
	} else if originalWidth >= 426 && originalHeight >= 240 {
		originalRes = "240p"
	}

	// If video is smaller than 240p, include at least 240p
	if originalRes == "" {
		return []string{"240p"}
	}

	// Include all resolutions up to and including original
	for _, res := range standardResolutions {
		applicable = append(applicable, res)
		if res == originalRes {
			break
		}
	}

	return applicable
}

func getVideoDuration(videoPath string) (int64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath)
	out, err := cmd.Output()

	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}

	// Parse the output to get the duration
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse video duration: %w", err)
	}

	// convert the duration to milliseconds
	return int64(duration * 1000), nil
}

// TranscodeVideo transcodes the video to the specified resolutions and stores the output files in a temporary directory.
func TranscodeVideo(videoKey string, s3Handler *S3Handler) (*Response, error) {
	var outputResolutions []string

	localVideoPath, err := s3Handler.DownloadVideo(videoKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	defer os.Remove(localVideoPath)

	videoDurations, err := getVideoDuration(localVideoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get video duration: %w", err)
	}
	log.Printf("Video duration: %d ms", videoDurations)

	// Get the video's original resolution
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", localVideoPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get video resolution: %w", err)
	}

	originalWidth, originalHeight, err := parseResolution(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse original resolution: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "transcoded-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	baseKey := filepath.Base(videoKey)
	baseKey = baseKey[:len(baseKey)-len(filepath.Ext(baseKey))]

	// Get applicable resolutions
	resolutions := getApplicableResolutions(originalWidth, originalHeight)
	if len(resolutions) == 0 {
		// Fallback to at least 240p if no resolutions were selected
		resolutions = []string{"240p"}
	}

	fmt.Printf("Original resolution: %dx%d, Selected resolutions: %v\n",
		originalWidth, originalHeight, resolutions)

	for _, resolution := range resolutions {
		scale, ok := resolutionMap[resolution]
		if !ok {
			continue // Skip invalid resolutions
		}

		_, _, err := parseResolution(strings.Replace(scale, ":", "x", 1))
		if err != nil {
			continue // Skip invalid resolution format
		}

		// Remove the size comparison check to allow upscaling
		// For very small videos, we want to upscale to at least 240p

		outputDir := filepath.Join(tempDir, resolution)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create resolution subdir: %w", err)
		}

		outputFile := filepath.Join(outputDir, "index.m3u8")
		var cmdOutput bytes.Buffer

		// Add force_original_aspect_ratio to maintain aspect ratio during scaling
		cmd := exec.Command("ffmpeg", "-i", localVideoPath,
			"-c:v", "libx264",
			"-vf", fmt.Sprintf("scale=%s:force_original_aspect_ratio=decrease,pad=ceil(iw/2)*2:ceil(ih/2)*2", scale),
			"-preset", "medium", // Changed from ultrafast to medium for better quality
			"-crf", "23", // Changed from 30 to 23 for better quality
			"-c:a", "aac",
			"-b:a", "128k",
			"-hls_time", "10",
			"-hls_playlist_type", "vod",
			"-hls_segment_filename", filepath.Join(outputDir, "segment_%03d.ts"),
			"-y", // Add -y to overwrite output files
			outputFile)
		cmd.Stderr = &cmdOutput

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("transcode failed for %s: %w\nffmpeg output: %s",
				resolution, err, cmdOutput.String())
		}

		// Upload transcoded files
		if err := uploadTranscodedFiles(s3Handler, outputDir, baseKey, resolution); err != nil {
			return nil, err
		}

		outputResolutions = append(outputResolutions, resolution)
	}

	if len(outputResolutions) == 0 {
		return nil, fmt.Errorf("no valid resolutions were processed")
	}

	return &Response{
		Resolutions: outputResolutions,
		Duration:    videoDurations,
	}, nil
}

func uploadTranscodedFiles(s3Handler *S3Handler, outputDir, baseKey, resolution string) error {
	// Upload main playlist
	s3OutputKey := fmt.Sprintf("transcoded/%s/%s/index.m3u8", baseKey, resolution)
	if err := s3Handler.UploadTranscodedFile(filepath.Join(outputDir, "index.m3u8"), s3OutputKey); err != nil {
		return fmt.Errorf("failed to upload playlist: %w", err)
	}

	// Upload segments
	segments, err := filepath.Glob(filepath.Join(outputDir, "segment_*.ts"))
	if err != nil {
		return fmt.Errorf("failed to list segments: %w", err)
	}

	for _, segment := range segments {
		segmentName := filepath.Base(segment)
		s3SegmentKey := fmt.Sprintf("transcoded/%s/%s/%s", baseKey, resolution, segmentName)
		if err := s3Handler.UploadTranscodedFile(segment, s3SegmentKey); err != nil {
			return fmt.Errorf("failed to upload segment %s: %w", segmentName, err)
		}
	}

	return nil
}
