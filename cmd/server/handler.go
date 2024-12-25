package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
)

type Handler struct {
	db                 *pgxpool.Pool
	mq                 *amqp091.Connection
	s3                 *s3.Client
	fs                 fs.FS
	jwtSignatureKey    ed25519.PrivateKey
	jwtVerificationKey ed25519.PublicKey

	staticHandler http.Handler

	badRequestPage          []byte
	notFoundPage            []byte
	internalServerErrorPage []byte
}

func NewHandler(db *pgxpool.Pool, mq *amqp091.Connection, s3Client *s3.Client, fsys fs.FS, jwtSignatureKey ed25519.PrivateKey, jwtVerificationKey ed25519.PublicKey) *Handler {
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
	h.badRequestPage, err = h.execute("error.tmpl", http.StatusBadRequest)
	if err != nil {
		panic(err)
	}
	h.notFoundPage, err = h.execute("error.tmpl", http.StatusNotFound)
	if err != nil {
		panic(err)
	}
	h.internalServerErrorPage, err = h.execute("error.tmpl", http.StatusInternalServerError)
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
		h.serveServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

type ExecuteBuildParams struct {
	TimeLocation *time.Location
	Build        *build.Build
}

func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	// Cookie token.
	const cookieToken = "token"
	tokenCookie, err := r.Cookie(cookieToken)
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s cookie: %w", cookieToken, err))
		return
	}
	token, err := parseAndValidateTokenFromCookie(r.Context(), h.db, h.jwtVerificationKey, tokenCookie)
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s cookie: %w", cookieToken, err))
		return
	}
	userID := token.UserID

	// Form value id.
	const formValueID = "id"
	id, err := uuid.Parse(r.FormValue(formValueID))
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s form value: %w", formValueID, err))
		return
	}

	// Form value time_location.
	const formValueTimeLocation = "time_location"
	timeLocation, err := time.LoadLocation(r.FormValue(formValueTimeLocation))
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s form value: %w", formValueTimeLocation, err))
		return
	}

	getter := &build.Getter{DB: h.db}
	b, err := getter.Get(r.Context(), &build.GetterGetParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, build.ErrNotFound) || errors.Is(err, build.ErrAccessDenied) {
			h.serveClientError(w, r, err)
		}
		h.serveServerError(w, r, err)
		return
	}

	component, err := h.execute("Build", ExecuteBuildParams{TimeLocation: timeLocation, Build: b})
	if err != nil {
		h.serveServerError(w, r, err)
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(component)
}

type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

func parseAndValidateTokenFromCookie(ctx context.Context, db *pgxpool.Pool, jwtVerificationKey ed25519.PublicKey, cookie *http.Cookie) (*Token, error) {
	err := cookie.Valid()
	if err != nil {
		return nil, err
	}
	return parseAndValidateToken(ctx, db, jwtVerificationKey, cookie.Value)
}

func parseAndValidateToken(ctx context.Context, db *pgxpool.Pool, jwtVerificationKey ed25519.PublicKey, s string) (*Token, error) {
	jwtToken, err := jwt.ParseWithClaims(
		s,
		&jwt.RegisteredClaims{},
		func(t *jwt.Token) (any, error) {
			return jwtVerificationKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
	)
	if err != nil {
		return nil, err
	}
	claims := jwtToken.Claims.(*jwt.RegisteredClaims)

	var id uuid.UUID
	if claims.ID == "" {
		return nil, errors.New("empty jti token claim")
	}
	id, err = uuid.Parse(claims.ID)
	if err != nil {
		return nil, fmt.Errorf("jti token claim: %w", err)
	}

	var userID uuid.UUID
	if claims.Subject == "" {
		return nil, errors.New("empty sub token claim")
	}
	userID, err = uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("sub token claim: %w", err)
	}

	var expiresAt time.Time
	if claims.ExpiresAt == nil {
		return nil, errors.New("empty exp token claim")
	}
	expiresAt = claims.ExpiresAt.Time

	token := &Token{
		ID:        id,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}

	revoked, err := revokedTokenExists(ctx, db, token.ID)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, errors.New("revoked token")
	}

	return token, nil
}

func revokedTokenExists(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM revoked_tokens
			WHERE id = $1
		)
	`
	args := []any{id}

	rows, _ := db.Query(ctx, query, args...)
	return pgx.CollectExactlyOneRow(rows, pgx.RowTo[bool])
}

func (h *Handler) serveClientError(w http.ResponseWriter, _ *http.Request, err error) {
	slog.Warn("client error", "err", err)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(h.badRequestPage)
}

func (h *Handler) serveServerError(w http.ResponseWriter, _ *http.Request, err error) {
	slog.Error("server error", "err", err)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(h.internalServerErrorPage)
}
