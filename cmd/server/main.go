package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/k11v/brick/internal/app"
)

type Config struct {
	Host string
	Port int

	PostgreSQLConnectionString string
	RabbitMQConnectionString   string
	MinIOConnectionString      string

	JWTSignatureKeyFile    string
	JWTVerificationKeyFile string
}

func main() {
	const envHost = "APP_HOST"
	host := os.Getenv(envHost)
	if host == "" {
		host = "127.0.0.1"
	}

	const envPort = "APP_PORT"
	port := 0
	portEnv := os.Getenv(envPort)
	if portEnv != "" {
		var err error
		port, err = strconv.Atoi(portEnv)
		if err != nil {
			exit(fmt.Errorf("%s env: %w", envPort, err))
		}
	}
	if port == 0 {
		port = 8080
	}

	const envPostgreSQLConnectionString = "APP_POSTGRESQL_CONNECTION_STRING"
	postgreSQLConnectionString := os.Getenv(envPostgreSQLConnectionString)
	if postgreSQLConnectionString == "" {
		exit(fmt.Errorf("%s env is empty", envPostgreSQLConnectionString))
	}

	const envRabbitMQConnectionString = "APP_RABBITMQ_CONNECTION_STRING"
	rabbitMQConnectionString := os.Getenv(envRabbitMQConnectionString)
	if rabbitMQConnectionString == "" {
		exit(fmt.Errorf("%s env is empty", envRabbitMQConnectionString))
	}

	const envMinIOConnectionString = "APP_MINIO_CONNECTION_STRING"
	minIOConnectionString := os.Getenv(envMinIOConnectionString)
	if minIOConnectionString == "" {
		exit(fmt.Errorf("%s env is empty", envMinIOConnectionString))
	}

	const envJWTSignatureKeyFile = "APP_JWT_SIGNATURE_KEY_FILE"
	jwtSignatureKeyFile := os.Getenv(envJWTSignatureKeyFile)
	if jwtSignatureKeyFile == "" {
		exit(fmt.Errorf("%s env is empty", envJWTSignatureKeyFile))
	}

	const envJWTVerificationKeyFile = "APP_JWT_VERIFICATION_KEY_FILE"
	jwtVerificationKeyFile := os.Getenv(envJWTVerificationKeyFile)
	if jwtVerificationKeyFile == "" {
		exit(fmt.Errorf("%s env is empty", envJWTVerificationKeyFile))
	}

	cfg := &Config{
		Host:                       host,
		Port:                       port,
		PostgreSQLConnectionString: postgreSQLConnectionString,
		RabbitMQConnectionString:   rabbitMQConnectionString,
		MinIOConnectionString:      minIOConnectionString,
		JWTSignatureKeyFile:        jwtSignatureKeyFile,
		JWTVerificationKeyFile:     jwtVerificationKeyFile,
	}

	err := run(cfg)
	if err != nil {
		exit(err)
	}

	exit(nil)
}

func run(cfg *Config) error {
	ctx := context.Background()

	postgresPool, err := app.NewPostgresPool(ctx, cfg.PostgreSQLConnectionString)
	if err != nil {
		return err
	}
	defer postgresPool.Close()

	amqpClient := app.NewAMQPClient(
		cfg.RabbitMQConnectionString,
		&app.AMQPQueueDeclareParams{Name: app.AMQPQueueBuildCreated},
	)

	s3Client := app.NewS3Client(cfg.MinIOConnectionString)

	server, err := NewServer(postgresPool, amqpClient, s3Client, cfg)
	if err != nil {
		return err
	}

	slog.Info("starting server", "addr", server.Addr)
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// exit calls os.Exit(0) or os.Exit(1) based on err.
func exit(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
