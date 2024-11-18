package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	transport "github.com/aws/smithy-go/endpoints"

	"github.com/k11v/brick/internal/app/build/operation"
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

// NewStorage creates a new Storage instance using the provided connection string.
// The connection string must be a valid URL in the format: http://key:secret@s3:9000.
// For MinIO, the key and secret are the username and password respectively.
// It panics if the connection string is not a valid URL.
func NewStorage(connectionString string) *Storage {
	u, err := url.Parse(connectionString)
	if err != nil {
		panic(err)
	}

	username := u.User.Username()
	password, _ := u.User.Password()
	u.User = nil

	client := s3.New(
		s3.Options{
			Credentials:        credentials.NewStaticCredentialsProvider(username, password, ""),
			EndpointResolverV2: &endpointResolver{BaseURL: u},
		},
	)
	return &Storage{
		client:           client,
		uploadPartSize:   10 * 1024 * 1024, // 10MB
		downloadPartSize: 10 * 1024 * 1024, // 10MB
	}
}

// UploadFiles implements operation.Storage.
func (s *Storage) UploadFiles(ctx context.Context, params *operation.StorageUploadFilesParams) error {
	uploader := manager.NewUploader(s.client, func(u *manager.Uploader) {
		u.PartSize = int64(s.uploadPartSize)
	})

	for {
		p, err := params.MultipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
			// return
		} else if err != nil {
			return fmt.Errorf("storage upload files: %w", err)
			// log.Fatal(err)
		}

		bucketName := "brick"
		objectKey := path.Join(params.BuildID.String(), p.FileName()) // FIXME: p.FileName() returns only the last component and is platform-dependent.

		_, err = uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: &bucketName,
			Key:    &objectKey,
			Body:   p,
		})
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" { // FIXME: consider error related to manager.MaxUploadParts.
			return operation.FileTooLarge
		} else if err != nil {
			return fmt.Errorf("storage upload files: %w", err)
		}
		// slurp, err := io.ReadAll(p)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// fmt.Printf("Part %q: %q\n", p.Header.Get("Foo"), slurp)

		err = s3.NewObjectExistsWaiter(s.client).Wait(
			ctx,
			&s3.HeadObjectInput{
				Bucket: &bucketName,
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

	bucketName := "brick"
	objectPrefix := params.BuildID.String() + "/"

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &bucketName,
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
				Bucket: &bucketName,
				Key:    object.Key,
			})
			if err != nil {
				return fmt.Errorf("storage download files: %w", err)
			}
		}
	}

	return nil
}

// endpointResolver implements s3.EndpointResolverV2.
// It resolves endpoints for S3-compatible object storage like MinIO.
type endpointResolver struct {
	BaseURL *url.URL // required
}

func (r *endpointResolver) ResolveEndpoint(_ context.Context, params s3.EndpointParameters) (transport.Endpoint, error) {
	u := *r.BaseURL
	u.Path += "/" + *params.Bucket
	return transport.Endpoint{URI: u}, nil
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
