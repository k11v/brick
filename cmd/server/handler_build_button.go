package main

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/build"
)

type ExecuteMainParams struct {
	Build *build.Build
	Files []*build.File
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
	unnextPart := func() error {
		if bufNext || bufEmpty {
			return errors.New("unnextPart buf already next or empty")
		}
		bufNext = true
		return nil
	}

	var files iter.Seq2[*build.CreatorCreateFileParams, error] = func(yield func(*build.CreatorCreateFileParams, error) bool) {
		fileLoopStopped := false

		for i := 0; !fileLoopStopped; i++ {
			var (
				name      string
				typString string
				data      io.Reader
			)

		PartLoop:
			for {
				part, err := nextPart()
				if err != nil {
					if err == io.EOF {
						fileLoopStopped = true
						break PartLoop
					}
					_ = yield(nil, fmt.Errorf("body: %w", err))
					return
				}

				formName := part.FormName()
				switch {
				// Form value files/*/name.
				case formName == fmt.Sprintf("files/%d/name", i):
					valueBytes, err := io.ReadAll(part)
					if err != nil {
						_ = yield(nil, fmt.Errorf("%s form value: %w", formName, err))
						return
					}
					name = string(valueBytes)
				// Form value files/*/type.
				case formName == fmt.Sprintf("files/%d/type", i):
					valueBytes, err := io.ReadAll(part)
					if err != nil {
						_ = yield(nil, fmt.Errorf("%s form value: %w", formName, err))
						return
					}
					typString = string(valueBytes)
				// Form value files/*/data.
				case formName == fmt.Sprintf("files/%d/data", i):
					data = part
					break PartLoop
				case strings.HasPrefix(formName, fmt.Sprintf("files/%d/", i+1)):
					err := unnextPart()
					if err != nil {
						_ = yield(nil, err)
						return
					}
					break PartLoop
				default:
					_ = yield(nil, fmt.Errorf("%s form name unknown or misplaced", formName))
					return
				}
			}

			typ, known := build.ParseFileType(typString)
			if !known {
				formName := fmt.Sprintf("files/%d/type", i)
				_ = yield(nil, fmt.Errorf("%s form value unknown", formName))
				return
			}

			file := &build.CreatorCreateFileParams{
				Name:       name,
				Type:       typ,
				DataReader: data,
			}
			if !yield(file, nil) {
				return
			}
		}
	}

	creator := &build.Creator{DB: h.db, MQ: h.mq, STG: h.s3, BuildsAllowed: 10}
	createdBuild, err := creator.Create(ctx, &build.CreatorCreateParams{
		UserID:         userID,
		Files:          files,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	getter := &build.Getter{DB: h.db, STG: h.s3}
	gotFiles, err := getter.GetFiles(ctx, &build.GetterGetParams{
		ID:     createdBuild.ID,
		UserID: userID,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	comp, err := h.execute("build_main", &ExecuteMainParams{
		Build: createdBuild,
		Files: gotFiles,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(comp)
}
