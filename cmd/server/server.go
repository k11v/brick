package main

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/app"
)

func NewServer(db *pgxpool.Pool, mq *app.AMQPClient, st *s3.Client, cfg *Config) (*http.Server, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	mux := http.NewServeMux()

	subLogger := slog.With("source", "http")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	return &http.Server{
		Addr:     addr,
		Handler:  mux,
		ErrorLog: subLogLogger,
	}, nil
}
