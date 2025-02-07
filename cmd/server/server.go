package main

import (
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/app"
)

func NewServer(db *pgxpool.Pool, mq *app.AMQPClient, st *s3.Client, staticFsys fs.FS, cfg *Config) (*http.Server, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	h := NewHandler(db, mq, st, staticFsys)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /static/", h.GetStatic)

	subLogger := slog.With("source", "http")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	return &http.Server{
		Addr:     addr,
		Handler:  mux,
		ErrorLog: subLogLogger,
	}, nil
}

type Handler struct {
	db         *pgxpool.Pool
	mq         *app.AMQPClient
	st         *s3.Client
	staticFsys fs.FS
}

func NewHandler(db *pgxpool.Pool, mq *app.AMQPClient, st *s3.Client, staticFsys fs.FS) *Handler {
	return &Handler{
		db:         db,
		mq:         mq,
		st:         st,
		staticFsys: staticFsys,
	}
}

func (h *Handler) GetStatic(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/static/", http.FileServerFS(h.staticFsys)).ServeHTTP(w, r)
}
