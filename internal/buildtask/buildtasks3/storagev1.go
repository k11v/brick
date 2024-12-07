package buildtasks3

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

	"github.com/k11v/brick/internal/buildtask"
	"github.com/k11v/brick/internal/run/runs3"
)

// UploadFiles implements buildtask.Storage.
// FIXME: p.FileName() returns only the last component and is platform-dependent when we want the full path.
// TODO: consider the error related to manager.MaxUploadParts when handling uploader.Upload.
// FIXME: When UploadFiles fails, it should clean up the files it has possibly already uploaded.
func (s *Storage) UploadFiles(ctx context.Context, params *buildtask.StorageUploadFilesParams) error {
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
			Bucket: &runs3.BucketName,
			Key:    &objectKey,
			Body:   p,
		})
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" {
			return buildtask.FileTooLarge
		} else if err != nil {
			return fmt.Errorf("storage upload files: %w", err)
		}

		err = s3.NewObjectExistsWaiter(s.client).Wait(
			ctx,
			&s3.HeadObjectInput{
				Bucket: &runs3.BucketName,
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

// DownloadFiles implements buildtask.Storage.
func (s *Storage) DownloadFiles(ctx context.Context, params *buildtask.StorageDownloadFilesParams) error {
	downloader := manager.NewDownloader(s.client, func(d *manager.Downloader) {
		d.PartSize = int64(s.downloadPartSize)
		d.Concurrency = 1
	})

	objectPrefix := params.BuildID.String() + "/"

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &runs3.BucketName,
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
				Bucket: &runs3.BucketName,
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
