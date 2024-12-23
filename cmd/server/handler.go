package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
)

type Handler struct {
	db *pgxpool.Pool
	mq *amqp091.Connection
	s3 *s3.Client
	fs fs.FS

	staticHandler http.Handler

	notFoundPage            []byte
	internalServerErrorPage []byte
}

func NewHandler(db *pgxpool.Pool, mq *amqp091.Connection, s3Client *s3.Client, fsys fs.FS) *Handler {
	h := &Handler{db: db, mq: mq, s3: s3Client, fs: fsys}

	h.staticHandler = http.StripPrefix("/static/", http.FileServerFS(fsys))

	var err error
	h.notFoundPage, err = h.execute("error.tmpl", 404)
	if err != nil {
		panic(err)
	}
	h.internalServerErrorPage, err = h.execute("error.tmpl", 500)
	if err != nil {
		panic(err)
	}

	return h
}

func (h *Handler) execute(name string, data any) ([]byte, error) {
	funcs := template.FuncMap{
		"time": func(loc *time.Location, t *time.Time) string {
			return t.In(loc).Format("2006-01-02 15:04")
		},
		"status": func(operation *build.Build) string {
			if operation.ExitCode == nil {
				return "Queued"
			}
			if *operation.ExitCode == 0 {
				return "Completed"
			}
			return "Failed"
		},
		"jsonObject": func(args ...any) (string, error) {
			o := make(map[string]any)
			if len(args)%2 != 0 {
				return "", errors.New("args length is not even")
			}
			for len(args) > 0 {
				var kAny, v any
				kAny, v, args = args[0], args[1], args[2:]
				k, ok := kAny.(string)
				if !ok {
					return "", errors.New("key is not a string")
				}
				o[k] = v
			}
			oBytes, err := json.Marshal(o)
			if err != nil {
				return "", err
			}
			return string(oBytes), nil
		},
	}

	buf := new(bytes.Buffer)
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(h.fs, "main.tmpl", "error.tmpl"))
	err := tmpl.ExecuteTemplate(buf, name, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *Handler) StaticFile(w http.ResponseWriter, r *http.Request) {
	h.staticHandler.ServeHTTP(w, r)
}

func (h *Handler) NotFoundPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(h.notFoundPage)
}

func (h *Handler) MainPage(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("main.tmpl", nil)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(h.internalServerErrorPage)
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	component, err := h.execute("Build", struct {
		TimeLocation *time.Location
		Operation    *build.Build
	}{
		TimeLocation: time.Now().Location(),
		Operation:    &build.Build{},
	},
	)
	if err != nil {
		slog.Error("", "err", err)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(h.internalServerErrorPage)
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(component)
}
