package buildtasks3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/k11v/brick/internal/buildtask"
	"github.com/k11v/brick/internal/run/runs3"
)

var _ buildtask.Storage = (*Storage)(nil)

type Storage struct {
	client *s3.Client

	// uploadPartSize should be greater than or equal 5MB.
	// See github.com/aws/aws-sdk-go-v2/feature/s3/manager.
	uploadPartSize int

	// downloadPartSize should be greater than or equal 5MB.
	// See github.com/aws/aws-sdk-go-v2/feature/s3/manager.
	downloadPartSize int
}

// NewStorage creates a new Storage using the provided connection string.
// It panics if the connection string is not a valid URL.
func NewStorage(connectionString string) *Storage {
	return &Storage{
		client:           runs3.NewClient(connectionString),
		uploadPartSize:   10 * 1024 * 1024, // 10MB
		downloadPartSize: 10 * 1024 * 1024, // 10MB
	}
}

type StorageUploadFileV2Params struct{}

type StorageUploadFileV2Result struct{}

func (s *Storage) UploadFileV2(ctx context.Context, params *StorageUploadFileV2Params) (*StorageUploadFileV2Result, error) {
	return nil, nil
}

type StorageDownloadFileV2Params struct{}

type StorageDownloadFileV2Result struct{}

func (s *Storage) DownloadFileV2(ctx context.Context, params *StorageDownloadFileV2Params) (*StorageDownloadFileV2Result, error) {
	return nil, nil
}

type StorageUploadDirV2Params struct{}

type StorageUploadDirV2Result struct{}

func (s *Storage) UploadDirV2(ctx context.Context, params *StorageUploadDirV2Params) (*StorageUploadDirV2Result, error) {
	return nil, nil
}

type StorageDownloadDirV2Params struct{}

type StorageDownloadDirV2Result struct{}

func (s *Storage) DownloadDirV2(ctx context.Context, params *StorageDownloadDirV2Params) (*StorageDownloadDirV2Result, error) {
	return nil, nil
}
