package amqp

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/k11v/brick/internal/build"
)

// TODO: Consider acknowledgement and not receiving a message again.
func TestBroker(t *testing.T) {
	t.Run("sends and receives build tasks", func(t *testing.T) {
		ctx := context.Background()
		broker := NewTestBroker(t, ctx)

		b := &build.Build{
			ID:             uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
			IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
			UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			CreatedAt:      time.Now().UTC(),
			DocumentToken:  "document token",
			DocumentFiles: map[string][]byte{
				"apple.md":  []byte("apples"),
				"banana.md": []byte("bananas"),
			},
			Status: "pending",
			Done:   false,
		}

		err := broker.SendBuildTask(ctx, b)
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		got, err := broker.ReceiveBuildTask(ctx)
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		if want := b; !reflect.DeepEqual(got, want) {
			t.Logf("got %v", got)
			t.Fatalf("want %v", want)
		}
	})
}

func NewTestBroker(tb testing.TB, ctx context.Context) *Broker {
	tb.Helper()

	username := "guest"
	password := "guest"

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "rabbitmq:4.0-alpine",
			Env: map[string]string{
				"RABBITMQ_DEFAULT_USER": username,
				"RABBITMQ_DEFAULT_PASS": password,
			},
			ExposedPorts: []string{"5672/tcp"},
			WaitingFor:   wait.ForLog(".*Server startup complete.*").AsRegexp().WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	testcontainers.CleanupContainer(tb, c)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}

	endpoint, err := c.PortEndpoint(ctx, nat.Port("5672/tcp"), "")
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}

	connectionString := fmt.Sprintf("amqp://%s:%s@%s", username, password, endpoint)

	return NewBroker(connectionString)
}
