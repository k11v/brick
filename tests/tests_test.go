package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/compose"
)

func TestServer(t *testing.T) {
	t.Run(`GET /health returns OK`, func(t *testing.T) {
		ctx := context.Background()
		baseURL := NewTestServer(t, ctx)

		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		t.Cleanup(func() {
			_ = resp.Body.Close()
		})

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		bodyString := string(body)

		if got, want := resp.StatusCode, http.StatusOK; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if got, want := bodyString, `{"status":"ok"}`; !EqualJSON(got, want) {
			t.Logf("got %s", got)
			t.Fatalf("want %s", want)
		}
	})
}

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

func EqualJSON(x, y string) bool {
	var mx, my any
	if err := json.Unmarshal([]byte(x), &mx); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(y), &my); err != nil {
		return false
	}
	return reflect.DeepEqual(mx, my)
}
