package main

import (
	"log"
	"net"
	"os"

	"video-transcoder-service/internal/transcoder"
	pb "video-transcoder-service/proto"

	"github.com/joho/godotenv"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedVideoTranscoderServiceServer
	s3Handler *transcoder.S3Handler
}

func (s *server) TranscodeVideo(stream pb.VideoTranscoderService_TranscodeVideoServer) error {
	var videoKey string

	for {
		req, err := stream.Recv()
		if err != nil {
			break
		}
		videoKey = req.GetFilename()
		log.Printf("Received video key %s", videoKey)
	}

	response, err := transcoder.TranscodeVideo(videoKey, s.s3Handler)
	if err != nil {
		return stream.SendAndClose(&pb.TranscodeVideoResponse{
			Message: err.Error(),
			Success: false,
		})
	}
	log.Printf("Transcoded files: %v", response.Resolutions)

	return stream.SendAndClose(&pb.TranscodeVideoResponse{
		Message:         "Transcoded successfully",
		Success:         true,
		TranscodedFiles: response.Resolutions,
		Duration:        response.Duration,
	})
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	// Initialize S3 handler
	s3Handler, err := transcoder.NewS3Handler(os.Getenv("AWS_DOWNLOAD_BUCKET_NAME"), os.Getenv("AWS_UPLOAD_BUCKET_NAME"))
	if err != nil {
		log.Fatalf("failed to create S3 handler: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterVideoTranscoderServiceServer(s, &server{s3Handler: s3Handler})
	log.Printf("server listening at %v", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
