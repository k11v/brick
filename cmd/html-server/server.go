package main

import (
	"bytes"
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

	"github.com/k11v/brick/internal/multifile"
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

		partReader := multipart.NewReader(r.Body, boundary)
		fileIndex := -1
		fileReader := multifile.NewReader(func() (name string, content io.Reader, err error) {
			fileIndex++

			namePart, err := partReader.NextPart()
			if err != nil {
				return "", nil, err
			}
			if namePart.FormName() != fmt.Sprintf("files/%d/name", fileIndex) {
				return "", nil, fmt.Errorf("unexpected %d file index", fileIndex)
			}
			nameBytes, err := io.ReadAll(namePart)
			if err != nil {
				return "", nil, err
			}

			contentPart, err := partReader.NextPart()
			if err != nil {
				return "", nil, err
			}
			if contentPart.FormName() != fmt.Sprintf("files/%d/content", fileIndex) {
				return "", nil, fmt.Errorf("unexpected %d file index", fileIndex)
			}

			return string(nameBytes), contentPart, nil
		})

		type File struct {
			Name    string
			Content io.Reader
		}

		var files iter.Seq2[*File, error] = func(yield func(*File, error) bool) {
			for fileReader.Read() {
				file := &File{
					Name:    fileReader.Name(),
					Content: fileReader.Content(),
				}
				if !yield(file, nil) {
					return
				}
			}
			if fileReader.Err() != nil {
				_ = yield(nil, fileReader.Err())
				return
			}
		}

		for file, err := range files {
			if err != nil {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}

			contentBuf := new(bytes.Buffer)
			_, err = io.Copy(contentBuf, file.Content)
			if err != nil {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}

			head := contentBuf.Bytes()[:min(contentBuf.Len(), 100)]
			slog.Info("received file", "name", file.Name, "content", string(head))
		}

		w.WriteHeader(http.StatusOK)
		err = writeTemplate(w, "build", nil, "main.tmpl")
		if err != nil {
			slog.Error("failed", "err", err)
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
