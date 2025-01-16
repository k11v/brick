package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/build"
)

type ExecuteErrorParams struct {
	StatusCode int
}

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
		"time": func(loc *time.Location, t *time.Time) string {
			return t.In(loc).Format("2006-01-02 15:04")
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
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(h.fs, "*.html.tmpl"))
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

type ExecuteBuildParams struct{}

func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("build.html.tmpl", &ExecuteBuildParams{})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

type ExecuteMainPageV1Params struct {
	Header *ExecuteHeaderParams
}

func (h *Handler) MainPageV1(w http.ResponseWriter, r *http.Request) {
	page, err := h.execute("main.html.tmpl", &ExecuteMainPageV1Params{
		Header: &ExecuteHeaderParams{
			User: nil,
		},
	})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

type ExecuteBuildV1Params struct {
	TimeLocation *time.Location
	Build        *build.Build
}

func (h *Handler) BuildV1(w http.ResponseWriter, r *http.Request) {
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

	getter := &build.Getter{DB: h.db, S3: h.s3}
	b, err := getter.Get(r.Context(), &build.GetterGetParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, build.ErrNotFound) || errors.Is(err, build.ErrAccessDenied) {
			h.serveClientError(w, r, err)
		} else {
			h.serveServerError(w, r, err)
		}
		return
	}

	buildHTML, err := h.execute("Build", &ExecuteBuildV1Params{TimeLocation: timeLocation, Build: b})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buildHTML)
}

func (h *Handler) BuildOutputFile(w http.ResponseWriter, r *http.Request) {
	// Header HX-Request.
	const headerHXRequest = "HX-Request"
	hxRequest, err := strconv.ParseBool(r.Header.Get(headerHXRequest))
	if err != nil {
		hxRequest = false
	}
	if hxRequest { // TODO: This works but seems a bit risky. What if it loops?
		w.Header().Set("HX-Redirect", r.URL.String())
		w.WriteHeader(http.StatusNoContent)
		return
	}

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

	buf := new(bytes.Buffer) // TODO: Avoid loading entire output file into memory.
	getter := &build.Getter{DB: h.db, S3: h.s3}
	err = getter.GetOutputFile(r.Context(), buf, &build.GetterGetParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, build.ErrNotFound) || errors.Is(err, build.ErrAccessDenied) || errors.Is(err, build.ErrNotDone) {
			h.serveClientError(w, r, err)
		} else {
			h.serveServerError(w, r, err)
		}
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=output.pdf")
	w.Header().Set("Content-Type", "application/pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

type ExecuteBuildLogParams struct {
	Content string
}

func (h *Handler) BuildLog(w http.ResponseWriter, r *http.Request) {
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

	buf := new(bytes.Buffer) // TODO: Avoid loading entire output file into memory.
	getter := &build.Getter{DB: h.db, S3: h.s3}
	err = getter.GetLogFile(r.Context(), buf, &build.GetterGetParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, build.ErrNotFound) || errors.Is(err, build.ErrAccessDenied) || errors.Is(err, build.ErrNotDone) {
			h.serveClientError(w, r, err)
		} else {
			h.serveServerError(w, r, err)
		}
		return
	}

	buildLogHTML, err := h.execute("BuildLog", &ExecuteBuildLogParams{Content: buf.String()})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buildLogHTML)
}

func (h *Handler) BuildFromBuild(w http.ResponseWriter, r *http.Request) {
	// Header Content-Type.
	const headerContentType = "Content-Type"
	mediaType, params, err := mime.ParseMediaType(r.Header.Get(headerContentType))
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s header: %w", headerContentType, err))
		return
	}
	if mediaType != "multipart/form-data" {
		h.serveClientError(w, r, fmt.Errorf("not multipart/form-data %s header", headerContentType))
		return
	}
	boundary := params["boundary"]
	if boundary == "" {
		h.serveClientError(w, r, fmt.Errorf("%s header with empty boundary", headerContentType))
		return
	}

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

	mr := multipart.NewReader(r.Body, boundary)
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

	// Form value time_location.
	const formValueTimeLocation = "time_location"
	timeLocationFormValue := ""
	if part, err := peekPart(); err == nil && part.FormName() == formValueTimeLocation {
		_, _ = nextPart()
		timeLocationBytes, err := io.ReadAll(part)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		timeLocationFormValue = string(timeLocationBytes)
	}
	timeLocation, err := time.LoadLocation(timeLocationFormValue)
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s form value: %w", formValueTimeLocation, err))
		return
	}

	// Form value files/*.
	var files iter.Seq2[*build.File, error] = func(yield func(*build.File, error) bool) {
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
					file := &build.File{Name: name, Data: bytes.NewReader(nil)}
					_ = yield(file, nil)
					continue
				}
			}
			contentPart, err := nextPart()
			if err != nil {
				if errors.Is(err, io.EOF) {
					file := &build.File{Name: name, Data: bytes.NewReader(nil)}
					_ = yield(file, nil)
					return
				}
				_ = yield(nil, err)
				return
			}

			file := &build.File{Name: name, Data: contentPart}
			if !yield(file, nil) {
				return
			}
		}
	}

	creator := &build.Creator{DB: h.db, MQ: h.mq, S3: h.s3, BuildsAllowed: 10}
	b, err := creator.Create(r.Context(), &build.CreatorCreateParams{
		UserID:         userID,
		Files:          files,
		IdempotencyKey: uuid.New(),
	})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	buildHTML, err := h.execute("Build", &ExecuteBuildV1Params{TimeLocation: timeLocation, Build: b})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buildHTML)
}

func (h *Handler) BuildFromCancel(w http.ResponseWriter, r *http.Request) {
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

	canceler := &build.Canceler{DB: h.db, MQ: h.mq, S3: h.s3}
	b, err := canceler.Cancel(r.Context(), &build.CancelerCancelParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, build.ErrNotFound) || errors.Is(err, build.ErrAccessDenied) || errors.Is(err, build.ErrAlreadyDone) {
			h.serveClientError(w, r, err)
		} else {
			h.serveServerError(w, r, err)
		}
		return
	}

	buildHTML, err := h.execute("Build", &ExecuteBuildV1Params{TimeLocation: timeLocation, Build: b})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buildHTML)
}

type ExecuteHeaderParams struct {
	User *struct{ ID uuid.UUID }
}

func (h *Handler) HeaderFromSignIn(w http.ResponseWriter, r *http.Request) {
	newUserID := uuid.New()

	newToken, err := createToken(h.jwtSignatureKey, newUserID)
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	headerHTML, err := h.execute("Header", &ExecuteHeaderParams{User: &struct{ ID uuid.UUID }{ID: newUserID}})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    newToken,
		Path:     "/",
		Domain:   "localhost",
		MaxAge:   int(14 * 24 * time.Hour / time.Second), // TODO: Consider time zones.
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(headerHTML)
}

func (h *Handler) HeaderFromSignOut(w http.ResponseWriter, r *http.Request) {
	// Cookie token.
	const cookieToken = "token"
	tokenCookie, err := r.Cookie(cookieToken)
	if err != nil {
		h.serveClientError(w, r, fmt.Errorf("%s cookie: %w", cookieToken, err))
		return
	}
	token, err := parseAndValidateTokenFromCookie(r.Context(), h.db, h.jwtVerificationKey, tokenCookie)
	if err != nil {
		// FIXME: parseAndValidateTokenFromCookie includes database interaction.
		// Any error from the interaction will directly reach the client since it
		// is served as a client error. This possibly exposes too much internals.
		h.serveClientError(w, r, fmt.Errorf("%s cookie: %w", cookieToken, err))
		return
	}

	err = createRevokedToken(r.Context(), h.db, token.ID, token.ExpiresAt)
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	headerHTML, err := h.execute("Header", &ExecuteHeaderParams{User: nil})
	if err != nil {
		h.serveServerError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{ // TODO: Check if all we need is Name and MaxAge.
		Name:     "token",
		Value:    "",
		Path:     "/",
		Domain:   "localhost",
		MaxAge:   -1, // Negative MaxAge deletes the cookie.
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(headerHTML)
}

type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

func createToken(jwtSignatureKey ed25519.PrivateKey, userID uuid.UUID) (string, error) {
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(14 * 24 * time.Hour)), // TODO: Consider time zones.
		IssuedAt:  jwt.NewNumericDate(time.Now()),                          // TODO: Consider time zones.
		ID:        uuid.NewString(),
	})
	return jwtToken.SignedString(jwtSignatureKey)
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

func createRevokedToken(ctx context.Context, db *pgxpool.Pool, id uuid.UUID, expiresAt time.Time) error {
	query := `
		INSERT INTO revoked_tokens (id, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (id) DO NOTHING
	`
	args := []any{id, expiresAt}

	_, err := db.Exec(ctx, query, args...) // TODO: Check if ignoring the command tag is OK.
	return err
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
