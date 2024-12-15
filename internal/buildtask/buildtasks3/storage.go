package buildtasks3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"mime/multipart"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/k11v/brick/internal/build"
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

func (s *Storage) UploadFileV2(ctx context.Context, key string, r io.Reader) error {
	return nil
}

type StorageDownloadFileV2Params struct{}

type StorageDownloadFileV2Result struct{}

func (s *Storage) DownloadFileV2(ctx context.Context, params *StorageDownloadFileV2Params) (*StorageDownloadFileV2Result, error) {
	return nil, nil
}

func (s *Storage) UploadDirV2(ctx context.Context, prefix string, files iter.Seq2[*build.File, error]) error {
	uploader := manager.NewUploader(s.client, func(u *manager.Uploader) {
		u.PartSize = int64(s.uploadPartSize)
	})

	for file, err := range files {
		if err != nil {
			return fmt.Errorf("buildtasks3.Storage: %w", err)
		}

		key := path.Join(prefix, file.Name)
		_, err = uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: &runs3.BucketName,
			Key:    &key,
			Body:   file.Content,
		})
		if err != nil {
			if apiErr := smithy.APIError(nil); errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" {
				err = errors.Join(buildtask.FileTooLarge, err)
			}
			return fmt.Errorf("buildtasks3.Storage: %w", err)
		}

		err = s3.NewObjectExistsWaiter(s.client).Wait(ctx, &s3.HeadObjectInput{
			Bucket: &runs3.BucketName,
			Key:    &key,
		}, time.Minute)
		if err != nil {
			return fmt.Errorf("buildtasks3.Storage: %w", err)
		}
	}

	return nil
}

func (s *Storage) DownloadDirV2(ctx context.Context, prefix string) (*multipart.Reader, error) {
	return nil, nil
}
