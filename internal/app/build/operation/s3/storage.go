package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/k11v/brick/internal/app/build/operation"
	apps3 "github.com/k11v/brick/internal/app/s3"
)

var _ operation.Storage = (*Storage)(nil)

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
		client:           apps3.NewClient(connectionString),
		uploadPartSize:   10 * 1024 * 1024, // 10MB
		downloadPartSize: 10 * 1024 * 1024, // 10MB
	}
}

// UploadFiles implements operation.Storage.
// FIXME: p.FileName() returns only the last component and is platform-dependent when we want the full path.
// TODO: consider the error related to manager.MaxUploadParts when handling uploader.Upload.
// FIXME: When UploadFiles fails, it should clean up the files it has possibly already uploaded.
func (s *Storage) UploadFiles(ctx context.Context, params *operation.StorageUploadFilesParams) error {
	uploader := manager.NewUploader(s.client, func(u *manager.Uploader) {
		u.PartSize = int64(s.uploadPartSize)
	})

	for {
		p, err := params.MultipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("storage upload files: %w", err)
		}

		objectKey := path.Join(params.BuildID.String(), p.FileName())

		_, err = uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: &apps3.BucketName,
			Key:    &objectKey,
			Body:   p,
		})
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" {
			return operation.FileTooLarge
		} else if err != nil {
			return fmt.Errorf("storage upload files: %w", err)
		}

		err = s3.NewObjectExistsWaiter(s.client).Wait(
			ctx,
			&s3.HeadObjectInput{
				Bucket: &apps3.BucketName,
				Key:    &objectKey,
			},
			time.Minute,
		)
		if err != nil {
			return fmt.Errorf("storage upload files: %w", err)
		}
	}

	return nil
}

// DownloadFiles implements operation.Storage.
func (s *Storage) DownloadFiles(ctx context.Context, params *operation.StorageDownloadFilesParams) error {
	downloader := manager.NewDownloader(s.client, func(d *manager.Downloader) {
		d.PartSize = int64(s.downloadPartSize)
		d.Concurrency = 1
	})

	objectPrefix := params.BuildID.String() + "/"

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &apps3.BucketName,
		Prefix: &objectPrefix,
	})

	fileNumber := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("storage download files: %w", err)
		}

		for _, object := range page.Contents {
			fileNumber++

			fileName, found := strings.CutPrefix(*object.Key, objectPrefix)
			if !found {
				panic("want prefix")
			}

			p, err := params.MultipartWriter.CreateFormFile(strconv.Itoa(fileNumber), fileName) // TODO: consider sending and receiving files in a tar archive
			if err != nil {
				return fmt.Errorf("storage download files: %w", err)
			}

			// fakeWriterAt needs manager.Downloader.Concurrency set to 1.
			_, err = downloader.Download(ctx, fakeWriterAt{p}, &s3.GetObjectInput{
				Bucket: &apps3.BucketName,
				Key:    object.Key,
			})
			if err != nil {
				return fmt.Errorf("storage download files: %w", err)
			}
		}
	}

	return nil
}

// fakeWriterAt wraps an io.Writer to provide a fake WriteAt method.
// This method simply calls w.Write ignoring the offset parameter.
// It can be used with github.com/aws/aws-sdk-go-v2/feature/s3/manager.Downloader.Download
// if its concurrency is set to 1 because this guarantees the sequential writes.
type fakeWriterAt struct {
	w io.Writer // required
}

func (writerAt fakeWriterAt) WriteAt(p []byte, _ int64) (n int, err error) {
	return writerAt.w.Write(p)
}
