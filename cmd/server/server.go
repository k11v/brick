package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/app"
)

func NewServer(db *pgxpool.Pool, mq *app.AMQPClient, st *s3.Client, staticFsys fs.FS, cfg *Config) (*http.Server, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	h := NewHandler(db, mq, st, staticFsys)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.GetRoot)
	mux.HandleFunc("GET /static/", h.GetStatic)
	mux.HandleFunc("GET /", h.GetDefault)

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

type ExecuteBuildParams struct{}

func (h *Handler) GetRoot(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("build.html.tmpl", &ExecuteBuildParams{})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	h.serveHTML(w, r, page)
}

func (h *Handler) GetStatic(w http.ResponseWriter, r *http.Request) {
	staticHandler := http.StripPrefix("/static/", http.FileServerFS(h.staticFsys))
	staticHandler.ServeHTTP(w, r)
}

type ExecuteErrorParams struct {
	StatusCode int
}

func (h *Handler) GetDefault(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("error.html.tmpl", &ExecuteErrorParams{
		StatusCode: http.StatusNotFound,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	h.serveHTMLWithStatusCode(w, r, page, http.StatusNotFound)
}

func (h *Handler) serveHTML(w http.ResponseWriter, r *http.Request, data []byte) {
	h.serveHTMLWithStatusCode(w, r, data, http.StatusOK)
}

func (h *Handler) serveHTMLWithStatusCode(w http.ResponseWriter, r *http.Request, data []byte, statusCode int) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(statusCode)
	_, _ = w.Write(data)
}

func (h *Handler) serveError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("error", "err", err)

	page, err := h.execute("error.html.tmpl", &ExecuteErrorParams{
		StatusCode: http.StatusInternalServerError,
	})
	if err != nil {
		panic(err)
	}
	h.serveHTMLWithStatusCode(w, r, page, http.StatusInternalServerError)
}

func (h *Handler) execute(name string, data any) ([]byte, error) {
	funcs := template.FuncMap{
		"now": func() string { // TODO: Remove.
			return time.Now().Format("2006-01-02 15:04:05")
		},
		"time": func(loc *time.Location, t *time.Time) string {
			return t.In(loc).Format("2006-01-02 15:04")
		},
		"uuid": func() string {
			return uuid.New().String()
		},
		"json": func(args ...any) (string, error) {
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
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(h.staticFsys, "*.html.tmpl"))
	err := tmpl.ExecuteTemplate(buf, name, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
