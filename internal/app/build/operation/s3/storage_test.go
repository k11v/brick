package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"testing"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/k11v/brick/internal/app/build/operation"
	apps3 "github.com/k11v/brick/internal/app/s3"
)

// TODO: Consider when upload doesn't have any files to upload.
// TODO: Consider when download doesn't have any files to download.
func TestStorage(t *testing.T) {
	t.Run("uploads and downloads files", func(t *testing.T) {
		ctx := context.Background()
		storage := NewTestStorage(t, ctx)

		// Prepare files.
		input := &bytes.Buffer{}
		mw := multipart.NewWriter(input)
		boundary := mw.Boundary()

		p, err := mw.CreateFormFile("1", "apple.md")
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		_, err = p.Write([]byte("apples"))
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		p, err = mw.CreateFormFile("2", "banana.md")
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		_, err = p.Write([]byte("bananas"))
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		if err = mw.Close(); err != nil {
			t.Fatalf("didn't want %q", err)
		}

		// Upload files.
		mr := multipart.NewReader(input, boundary)

		err = storage.UploadFiles(ctx, &operation.StorageUploadFilesParams{
			BuildID:         uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
			MultipartReader: mr,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		// Download files.
		output := &bytes.Buffer{}
		mw = multipart.NewWriter(output)
		boundary = mw.Boundary()

		err = storage.DownloadFiles(ctx, &operation.StorageDownloadFilesParams{
			BuildID:         uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
			MultipartWriter: mw,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		if err = mw.Close(); err != nil {
			t.Fatalf("didn't want %q", err)
		}

		// Check files.
		mr = multipart.NewReader(output, boundary)

		part, err := mr.NextPart()
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		if part.FormName() != "1" || part.FileName() != "apple.md" {
			t.Fatalf("didn't want %s %s", part.FormName(), part.FileName())
		}
		content := new(bytes.Buffer)
		if _, err = content.ReadFrom(part); err != nil {
			t.Fatalf("didn't want %q", err)
		}
		if content.String() != "apples" {
			t.Fatalf("didn't want %s", content.String())
		}

		part, err = mr.NextPart()
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		if part.FormName() != "2" || part.FileName() != "banana.md" {
			t.Fatalf("didn't want %s %s", part.FormName(), part.FileName())
		}
		content = new(bytes.Buffer)
		if _, err = content.ReadFrom(part); err != nil {
			t.Fatalf("didn't want %q", err)
		}
		if content.String() != "bananas" {
			t.Fatalf("didn't want %s", content.String())
		}

		if _, err = mr.NextPart(); !errors.Is(err, io.EOF) {
			t.Fatalf("didn't want %q", err)
		}
	})
}

func NewTestStorage(tb testing.TB, ctx context.Context) *Storage {
	tb.Helper()

	username := "minioadmin"
	password := "minioadmin"

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "quay.io/minio/minio:latest",
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor:   wait.ForHTTP("/minio/health/live").WithPort("9000"),
			Env: map[string]string{
				"MINIO_ROOT_USER":     username,
				"MINIO_ROOT_PASSWORD": password,
			},
			Cmd: []string{"server", "/data"},
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	testcontainers.CleanupContainer(tb, c)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}
	port, err := c.MappedPort(ctx, "9000/tcp")
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}
	connectionString := fmt.Sprintf("http://%s:%s@%s:%s", username, password, host, port.Port())

	client := apps3.NewClient(connectionString)
	err = apps3.Setup(ctx, client)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}

	return NewStorage(connectionString)
}
