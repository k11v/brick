package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/amqputil"
)

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

func (h *Handler) serveError(w http.ResponseWriter, _ *http.Request, err error) {
	slog.Warn("client or server error", "err", err)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(h.internalServerErrorPage)
}

func (h *Handler) NotFoundPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(h.notFoundPage)
}

func (h *Handler) StaticFile(w http.ResponseWriter, r *http.Request) {
	h.staticHandler.ServeHTTP(w, r)
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
