package apptest

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/k11v/brick/internal/app"
)

func SetupPostgres(ctx context.Context) (connectionString string, teardown func() error, err error) {
	db := "postgres"
	password := "postgres"
	user := "postgres"

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:17-alpine",
			Env: map[string]string{
				"POSTGRES_DB":       db,
				"POSTGRES_PASSWORD": password,
				"POSTGRES_USER":     user,
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			).WithDeadline(60 * time.Second),
		},
		Started: true,
	}
	postgresContainer, err := testcontainers.GenericContainer(ctx, req)
	maybeTeardown := func() error {
		return testcontainers.TerminateContainer(postgresContainer)
	}
	defer func() {
		if maybeTeardown != nil {
			maybeTeardown()
		}
	}()
	if err != nil {
		return "", nil, err
	}

	host, err := postgresContainer.Host(ctx)
	if err != nil {
		return "", nil, err
	}
	port, err := postgresContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return "", nil, err
	}
	connectionString = fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		user,
		password,
		net.JoinHostPort(host, port.Port()),
		db,
	)

	if err = app.SetupPostgres(connectionString); err != nil {
		return "", nil, err
	}

	teardown = maybeTeardown
	maybeTeardown = nil
	return connectionString, teardown, nil
}
