package s3

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestStorage(t *testing.T) {
	t.Run("uploads and downloads files", func(t *testing.T) {
		ctx := context.Background()
		storage := NewTestStorage(t, ctx)
		_ = storage
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
	connectionString := fmt.Sprintf("%s:%s", host, port.Port())

	return NewStorage(connectionString)
}
