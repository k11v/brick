package main

import (
	"fmt"
	"io"
	"iter"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/build"
)

type ExecuteMainParams struct {
	Build *build.Build
	Files *ExecuteFilesParams
}

// BuildButtonClickToMain.
// For status code 400, it responds with build_error.
func (h *Handler) BuildButtonClickToMain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Header X-Idempotency-Key.
	const headerIdempotencyKey = "X-Idempotency-Key"
	idempotencyKeyHeader := r.Header.Get(headerIdempotencyKey)
	if idempotencyKeyHeader == "" {
		h.serveError(w, r, fmt.Errorf("%s header empty or missing", headerIdempotencyKey))
		return
	}
	idempotencyKey, err := uuid.Parse(idempotencyKeyHeader)
	if err != nil {
		h.serveError(w, r, fmt.Errorf("%s header: %w", headerIdempotencyKey, err))
		return
	}

	// Cookie access_token.
	const cookieAccessToken = "access_token"
	accessTokenCookie, err := r.Cookie(cookieAccessToken)
	if err != nil {
		h.serveError(w, r, fmt.Errorf("%s cookie: %w", cookieAccessToken, err))
		return
	}
	accessToken, err := parseAndValidateTokenFromCookie(ctx, h.db, h.jwtVerificationKey, accessTokenCookie)
	if err != nil {
		h.serveError(w, r, fmt.Errorf("%s cookie: %w", cookieAccessToken, err))
		return
	}
	userID := accessToken.UserID

	mr, err := r.MultipartReader()
	if err != nil {
		h.serveError(w, r, fmt.Errorf("request: %w", err))
		return
	}

	var (
		bufPart  *multipart.Part
		bufErr   error
		bufNext  = false
		bufEmpty = true
	)
	nextPart := func() (*multipart.Part, error) {
		if bufNext {
			bufNext = false
			return bufPart, bufErr
		}
		bufPart, bufErr = mr.NextPart()
		bufEmpty = false
		return bufPart, bufErr
	}
	mustUnnextPart := func() {
		if bufNext || bufEmpty {
			panic("mustUnnextPart: buf already next or empty")
		}
		bufNext = true
	}

	var filesParams iter.Seq2[*build.CreatorCreateFileParams, error] = func(yield func(*build.CreatorCreateFileParams, error) bool) {
	FileLoop:
		for i := 0; ; i++ {
			var (
				name       string
				typString  string
				dataReader io.Reader
				ok         bool
			)

		PartLoop:
			for {
				part, err := nextPart()
				if err != nil {
					if err == io.EOF {
						mustUnnextPart()
						break PartLoop
					}
					h.serveError(w, r, fmt.Errorf("body: %w", err))
					return
				}

				formName := part.FormName()
				switch {
				// Form value files/*/name.
				case formName == fmt.Sprintf("files/%d/name", i):
					valueBytes, err := io.ReadAll(part)
					if err != nil {
						h.serveError(w, r, fmt.Errorf("%s form value: %w", formName, err))
						return
					}
					name = string(valueBytes)
					ok = true
				// Form value files/*/type.
				case formName == fmt.Sprintf("files/%d/type", i):
					valueBytes, err := io.ReadAll(part)
					if err != nil {
						h.serveError(w, r, fmt.Errorf("%s form value: %w", formName, err))
						return
					}
					typString = string(valueBytes)
					ok = true
				// Form value files/*/data.
				case formName == fmt.Sprintf("files/%d/data", i):
					dataReader = part
					ok = true
					break PartLoop
				case strings.HasPrefix(formName, fmt.Sprintf("files/%d/", i+1)):
					mustUnnextPart()
					break PartLoop
				default:
					h.serveError(w, r, fmt.Errorf("%s form name unknown or misplaced", formName))
					return
				}
			}
			if !ok {
				break FileLoop
			}

			name = path.Join("/", name)

			typ, known := build.ParseFileType(typString)
			if !known {
				formName := fmt.Sprintf("files/%d/type", i)
				h.serveError(w, r, fmt.Errorf("%s form value unknown", formName))
				return
			}

			file := &build.CreatorCreateFileParams{
				Name:       name,
				Type:       typ,
				DataReader: dataReader,
			}
			if !yield(file, nil) {
				return
			}
		}
	}

	creator := build.NewCreator(h.db, h.mq, h.s3, &build.CreatorParams{BuildsAllowed: 10})
	b, err := creator.Create(ctx, &build.CreatorCreateParams{
		UserID:         userID,
		Files:          filesParams,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	getter := build.NewGetter(h.db, h.s3)
	files, err := getter.GetFiles(ctx, &build.GetterGetParams{
		ID:     b.ID,
		UserID: userID,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	executeFilesParams, err := ExecuteFilesParamsFromFiles(files)
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	comp, err := h.execute("build_main", &ExecuteMainParams{
		Build: b,
		Files: executeFilesParams,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(comp)
}

func ExecuteFilesParamsFromFiles(files []*build.File) (*ExecuteFilesParams, error) {
	listFiles := make([]*ListFile, 0)
	for _, f := range files {
		listFiles = append(listFiles, &ListFile{Name: f.Name, Type: f.Type})
	}
	treeFiles, err := TreeFilesFromListFiles(listFiles)
	if err != nil {
		return nil, err
	}
	return &ExecuteFilesParams{TreeFiles: treeFiles}, nil
}
