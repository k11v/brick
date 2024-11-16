package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestServer(t *testing.T) {
	ctx := context.Background()
	baseURL, teardown, err := SetupServer(ctx)
	if err != nil {
		t.Fatalf("didn't want %q", err)
	}
	t.Cleanup(func() {
		if err := teardown(); err != nil {
			t.Errorf("didn't want %q", err)
		}
	})

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("didn't want %q", err)
	}
	defer resp.Body.Close()

	health, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("didn't want %q", err)
	}

	if got, want := string(health), `{"status":"ok"}`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func SetupServer(ctx context.Context) (baseURL string, teardown func() error, err error) {
	teardownFuncs := make([]func() error, 0)
	maybeTeardown := func() error {
		var merr error
		for len(teardownFuncs) > 0 {
			var teardownFunc func() error
			teardownFuncs, teardownFunc = teardownFuncs[:len(teardownFuncs)-1], teardownFuncs[len(teardownFuncs)-1]

			if terr := teardownFunc(); terr != nil {
				merr = errors.Join(merr, terr)
			}
		}
		return merr
	}
	defer func() {
		if maybeTeardown != nil {
			maybeTeardown()
		}
	}()

	clusterNetwork, err := network.New(ctx)
	if err != nil {
		return "", nil, err
	}
	teardownFuncs = append(teardownFuncs, func() error {
		return clusterNetwork.Remove(ctx)
	})

	// FIXME: This connection string won't work.
	postgresConnectionString := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

	serverContainerReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:       "../.",
				PrintBuildLog: true,
			},
			Env: map[string]string{
				"BRICK_POSTGRES_DSN": postgresConnectionString,
				"BRICK_SERVER_HOST":  "0.0.0.0",
				"BRICK_SERVER_PORT":  "8080",
			},
			ExposedPorts: []string{"8080/tcp"},
			Networks: []string{
				clusterNetwork.Name,
			},
			NetworkAliases: map[string][]string{
				clusterNetwork.Name: {"server"},
			},
			WaitingFor: wait.ForAll(
				wait.ForHTTP("GET /health").WithPort("8080/tcp"),
			).WithDeadline(60 * time.Second),
		},
		Started: true,
	}
	serverContainer, err := testcontainers.GenericContainer(ctx, serverContainerReq)
	teardownFuncs = append(teardownFuncs, func() error {
		return testcontainers.TerminateContainer(serverContainer)
	})
	if err != nil {
		return "", nil, err
	}

	host, err := serverContainer.Host(ctx)
	if err != nil {
		return "", nil, err
	}

	var port string
	mappedPort, err := serverContainer.MappedPort(ctx, "8080/tcp")
	if err == nil {
		port = mappedPort.Port()
	} else {
		return "", nil, err
	}

	baseURL = fmt.Sprintf("http://%s", net.JoinHostPort(host, port))

	teardown = maybeTeardown
	maybeTeardown = nil
	return baseURL, teardown, nil
}
