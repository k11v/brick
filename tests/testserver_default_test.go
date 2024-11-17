//go:build !local

package tests

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/compose"
)

func NewTestServer(tb testing.TB, ctx context.Context) (baseURL string) {
	tb.Helper()

	baseURL, teardown, err := SetupServer(ctx)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}
	tb.Cleanup(func() {
		_ = teardown()
	})
	return baseURL
}

func SetupServer(ctx context.Context) (baseURL string, teardown func() error, err error) {
	project, err := compose.NewDockerCompose("../compose.yaml")
	if err != nil {
		return "", nil, err
	}
	maybeTeardown := func() error {
		return project.Down(ctx, compose.RemoveImagesLocal, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
	}
	defer func() {
		if maybeTeardown != nil {
			_ = maybeTeardown()
		}
	}()

	if err = project.Up(ctx, compose.Wait(true)); err != nil {
		return "", nil, err
	}

	serverContainer, err := project.ServiceContainer(ctx, "server")
	if err != nil {
		return "", nil, err
	}

	host, err := serverContainer.Host(ctx)
	if err != nil {
		return "", nil, err
	}

	var port string
	mappedPort, err := serverContainer.MappedPort(ctx, "8080/tcp")
	if err != nil {
		return "", nil, err
	}
	port = mappedPort.Port()

	baseURL = fmt.Sprintf("http://%s", net.JoinHostPort(host, port))

	teardown = maybeTeardown
	maybeTeardown = nil
	return baseURL, teardown, nil
}
