package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"iter"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
	"github.com/k11v/brick/internal/run/runpg"
	"github.com/k11v/brick/internal/run/runs3"
)

var templateFuncs = make(template.FuncMap)

func newServer(conf *config) *http.Server {
	addr := net.JoinHostPort(conf.host(), strconv.Itoa(conf.port()))

	subLogger := slog.Default().With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	mux := &http.ServeMux{}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := writeTemplate(w, "", nil, "main.tmpl")
		if err != nil {
			slog.Error("failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			err = writeErrorPage(w, http.StatusInternalServerError)
			if err != nil {
				slog.Error("failed to write error page", "err", err)
			}
		}
	})
	mux.HandleFunc("POST /build-create-form", func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		if mediaType != "multipart/form-data" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		boundary := params["boundary"]
		if boundary == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		mr := multipart.NewReader(r.Body, boundary)
		var files iter.Seq2[*build.File, error] = func(yield func(*build.File, error) bool) {
			peekedPart, peekedErr, peeked := (*multipart.Part)(nil), error(nil), false
			nextPart := func() (*multipart.Part, error) {
				if peeked {
					peeked = false
					return peekedPart, peekedErr
				}
				return mr.NextPart()
			}
			peekPart := func() (*multipart.Part, error) {
				if peeked {
					return peekedPart, peekedErr
				}
				peekedPart, peekedErr = mr.NextPart()
				peeked = true
				return peekedPart, peekedErr
			}

			for fileIndex := 0; ; fileIndex++ {
				namePart, err := nextPart()
				if err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					_ = yield(nil, err)
					return
				}
				if namePart.FormName() != fmt.Sprintf("files/%d/name", fileIndex) {
					_ = yield(nil, fmt.Errorf("want file index %d", fileIndex))
					return
				}
				nameBytes, err := io.ReadAll(namePart)
				if err != nil {
					_ = yield(nil, err)
					return
				}
				name := string(nameBytes)

				nameOrContentPart, err := peekPart()
				if err == nil {
					if nameOrContentPart.FormName() != fmt.Sprintf("files/%d/content", fileIndex) {
						file := &build.File{Name: name, Content: bytes.NewReader(nil)}
						_ = yield(file, nil)
						continue
					}
				}
				contentPart, err := nextPart()
				if err != nil {
					if errors.Is(err, io.EOF) {
						file := &build.File{Name: name, Content: bytes.NewReader(nil)}
						_ = yield(file, nil)
						return
					}
					_ = yield(nil, err)
					return
				}

				file := &build.File{Name: name, Content: contentPart}
				if !yield(file, nil) {
					return
				}
			}
		}

		db, err := runpg.NewPool(r.Context(), "postgres://postgres:postgres@127.0.0.1:5432/postgres")
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer db.Close()

		mq, err := amqp091.Dial("amqp://guest:guest@127.0.0.1:5672/")
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = mq.Close()
		}()

		s3Client := runs3.NewClient("http://minioadmin:minioadmin@127.0.0.1:9000")

		operationService := build.NewOperationCreator(db, mq, s3Client, 10)
		operation, err := operationService.Create(r.Context(), &build.OperationCreatorCreateParams{
			UserID:         uuid.New(),
			Files:          files,
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = operation

		w.WriteHeader(http.StatusOK)
		err = writeTemplate(w, "build", nil, "main.tmpl")
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			err = writeErrorPage(w, http.StatusInternalServerError)
			if err != nil {
				slog.Error("failed to write error page", "err", err)
			}
		}
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(dataFS)))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		err := writeErrorPage(w, http.StatusNotFound)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = writeErrorPage(w, http.StatusInternalServerError)
			if err != nil {
				slog.Error("failed to write error page", "err", err)
			}
		}
	})

	return &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           mux,
		ReadHeaderTimeout: conf.ReadHeaderTimeout,
	}
}

// writeTemplate.
// The first template name is the one being executed.
func writeTemplate(w io.Writer, name string, data any, templateFiles ...string) error {
	if name == "" && len(templateFiles) != 0 {
		name = filepath.Base(templateFiles[0])
	}
	tmpl := template.Must(
		template.New(name).Funcs(templateFuncs).ParseFS(dataFS, templateFiles...),
	)
	if err := tmpl.Execute(w, data); err != nil {
		return err
	}
	return nil
}

func writeErrorPage(w io.Writer, statusCode int) error {
	return writeTemplate(w, "", statusCode, "error.tmpl")
}
