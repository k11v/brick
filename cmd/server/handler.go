package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/amqputil"
	"github.com/k11v/brick/internal/build"
)

type fileWithoutData struct {
	Name string
	Type string
}

type ExecuteErrorParams struct {
	StatusCode int
}

type Handler struct {
	db                 *pgxpool.Pool
	mq                 *amqputil.Client
	s3                 *s3.Client
	fs                 fs.FS
	jwtSignatureKey    ed25519.PrivateKey
	jwtVerificationKey ed25519.PublicKey

	staticHandler http.Handler

	badRequestPage          []byte
	notFoundPage            []byte
	internalServerErrorPage []byte
}

func NewHandler(db *pgxpool.Pool, mq *amqputil.Client, s3Client *s3.Client, fsys fs.FS, jwtSignatureKey ed25519.PrivateKey, jwtVerificationKey ed25519.PublicKey) *Handler {
	h := &Handler{
		db:                 db,
		mq:                 mq,
		s3:                 s3Client,
		fs:                 fsys,
		jwtSignatureKey:    jwtSignatureKey,
		jwtVerificationKey: jwtVerificationKey,
	}

	h.staticHandler = http.StripPrefix("/static/", http.FileServerFS(fsys))

	var err error
	h.badRequestPage, err = h.execute("error.html.tmpl", &ExecuteErrorParams{StatusCode: http.StatusBadRequest})
	if err != nil {
		panic(err)
	}
	h.notFoundPage, err = h.execute("error.html.tmpl", &ExecuteErrorParams{StatusCode: http.StatusNotFound})
	if err != nil {
		panic(err)
	}
	h.internalServerErrorPage, err = h.execute("error.html.tmpl", &ExecuteErrorParams{StatusCode: http.StatusInternalServerError})
	if err != nil {
		panic(err)
	}

	return h
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
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(h.fs, "*.html.tmpl"))
	err := tmpl.ExecuteTemplate(buf, name, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *Handler) AccessTokenCookieSetter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		const cookieAccessToken = "access_token"
		accessTokenCookieExistsAndValid := false
		accessTokenCookie, err := r.Cookie(cookieAccessToken)
		if err == nil {
			_, err = parseAndValidateTokenFromCookie(ctx, h.db, h.jwtVerificationKey, accessTokenCookie)
			if err == nil {
				accessTokenCookieExistsAndValid = true
			}
		}

		if !accessTokenCookieExistsAndValid {
			newUserID := uuid.New()
			newToken, err := createToken(h.jwtSignatureKey, newUserID)
			if err != nil {
				h.serveServerError(w, r, err)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     cookieAccessToken,
				Value:    newToken,
				Path:     "/",
				Domain:   "localhost",
				MaxAge:   int(14 * 24 * time.Hour / time.Second),
				Secure:   true,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		next.ServeHTTP(w, r)
	})
}

type ExecutePageParams struct {
	mainParams *ExecuteMainParams
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("build.html.tmpl", &ExecutePageParams{
		mainParams: &ExecuteMainParams{},
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

type ExecuteDocumentParams struct {
	DirEntries []*DirEntry
}

type DirEntry struct {
	Name       string
	Type       string
	DirEntries []*DirEntry
}

func (h *Handler) DocumentFromChange(w http.ResponseWriter, r *http.Request) {
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

	files := make([]*fileWithoutData, 0)

FileLoop:
	for i := 0; ; i++ {
		var (
			name string
			typ  string
		)

	PartLoop:
		for {
			part, err := nextPart()
			if err != nil {
				if err == io.EOF {
					break FileLoop
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
			// Form value files/*/type.
			case formName == fmt.Sprintf("files/%d/type", i):
				valueBytes, err := io.ReadAll(part)
				if err != nil {
					h.serveError(w, r, fmt.Errorf("%s form value: %w", formName, err))
					return
				}
				typ = string(valueBytes)
			case strings.HasPrefix(formName, fmt.Sprintf("files/%d/", i+1)):
				err := unnextPart()
				if err != nil {
					h.serveError(w, r, err)
					return
				}
				break PartLoop
			default:
				h.serveError(w, r, fmt.Errorf("%s form name unknown or misplaced", formName))
				return
			}
		}

		file := &fileWithoutData{
			Name: name,
			Type: typ,
		}
		files = append(files, file)
	}

	dirEntryFromName := make(map[string]*DirEntry)
	dirEntryFromName["/"] = &DirEntry{
		Name:       path.Base("/"),
		Type:       "directory",
		DirEntries: nil,
	}

	for i, file := range files {
		if file.Name == "" {
			formName := fmt.Sprintf("files/%d/name", i)
			h.serveError(w, r, fmt.Errorf("%s form value empty or missing", formName))
			return
		}
		name := path.Join("/", file.Name)

		if file.Type == "" {
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value empty or missing", formName))
			return
		}
		switch file.Type {
		case "file", "directory":
		default:
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value unknown", formName))
			return
		}
		typ := file.Type

		if dirEntryFromName[name] != nil {
			formName := fmt.Sprintf("files/%d/name", i)
			h.serveError(w, r, fmt.Errorf("%s form value already exists", formName))
			return
		}
		dirEntryFromName[name] = &DirEntry{
			Name:       path.Base(name),
			Type:       typ,
			DirEntries: nil,
		}

		parentName := path.Dir(name)
		if dirEntryFromName[parentName] == nil {
			formName := fmt.Sprintf("files/%d/name", i)
			h.serveError(w, r, fmt.Errorf("%s form value not found", formName))
			return
		}
		if dirEntryFromName[parentName].Type != "directory" {
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value not directory", formName))
			return
		}
		dirEntryFromName[parentName].DirEntries = append(
			dirEntryFromName[parentName].DirEntries,
			dirEntryFromName[name],
		)
	}

	comp, err := h.execute("build_document", &ExecuteDocumentParams{
		DirEntries: dirEntryFromName["/"].DirEntries,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(comp)
}

type ExecuteMainParams struct {
	Build *build.Build
	Files []*build.File
}

// MainFromBuildButtonClick.
// For status code 400, it responds with build_error.
func (h *Handler) MainFromBuildButtonClick(w http.ResponseWriter, r *http.Request) {
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
	FileLoop:
		for i := 0; ; i++ {
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
						break FileLoop
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

func (h *Handler) DocumentFromClearButtonClick(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func (h *Handler) Main(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func (h *Handler) MainFromCancelButtonClick(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

// MainFromNewButtonClick responds with build_main for a new build
// and cancels the current build if it is still cancelable.
func (h *Handler) MainFromNewButtonClick(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func (h *Handler) OutputFileFromDownloadButtonClick(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func (h *Handler) NotFoundPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(h.notFoundPage)
}

func (h *Handler) StaticFile(w http.ResponseWriter, r *http.Request) {
	h.staticHandler.ServeHTTP(w, r)
}

func (h *Handler) serveError(w http.ResponseWriter, _ *http.Request, err error) {
	slog.Warn("client or server error", "err", err)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(h.internalServerErrorPage)
}

// mustMatch reports whether name matches the shell pattern.
// It panics only when when pattern is malformed.
func mustMatch(pattern string, name string) (matched bool) {
	matched, err := path.Match(pattern, name)
	if err != nil {
		panic(err)
	}
	return matched
}
