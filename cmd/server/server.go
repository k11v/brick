package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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
	"os"
	"path/filepath"
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

func NewServer(db *pgxpool.Pool, mq *amqp091.Connection, s3Client *s3.Client, conf *Config) (*http.Server, error) {
	addr := net.JoinHostPort(conf.host(), strconv.Itoa(conf.port()))

	subLogger := slog.With("component", "server")
	subLogLogger := slog.NewLogLogger(subLogger.Handler(), slog.LevelError)

	jwtSignatureKey, err := readFileWithED25519PrivateKey(conf.JWTSignatureKeyFile)
	if err != nil {
		return nil, fmt.Errorf("NewServer: %w", err)
	}
	jwtVerificationKey, err := readFileWithED25519PublicKey(conf.JWTVerificationKeyFile)
	if err != nil {
		return nil, fmt.Errorf("NewServer: %w", err)
	}
	h := NewHandler(db, mq, s3Client, dataFS, jwtSignatureKey, jwtVerificationKey)

	mux := &http.ServeMux{}

	mux.HandleFunc("GET /", h.NotFoundPage)
	mux.HandleFunc("GET /static/", h.StaticFile)

	mux.HandleFunc("GET /{$}", h.Page)
	mux.HandleFunc("POST /build_documentFromChange", h.DocumentFromChange)
	mux.HandleFunc("POST /build_mainFromBuildButtonClick", h.MainFromBuildButtonClick)

	mux.HandleFunc("GET /Build", h.BuildV1)
	mux.HandleFunc("GET /BuildOutputFile", h.BuildOutputFile)
	mux.HandleFunc("GET /BuildLog", h.BuildLog)
	mux.HandleFunc("POST /BuildFromBuild", h.BuildFromBuild)
	mux.HandleFunc("POST /BuildFromCancel", h.BuildFromCancel)
	mux.HandleFunc("POST /HeaderFromSignIn", h.HeaderFromSignIn)
	mux.HandleFunc("POST /HeaderFromSignOut", h.HeaderFromSignOut)

	muxWithMiddlewares := h.AccessTokenCookieSetter(mux)

	server := &http.Server{
		Addr:              addr,
		ErrorLog:          subLogLogger,
		Handler:           muxWithMiddlewares,
		ReadHeaderTimeout: conf.ReadHeaderTimeout,
	}
	return server, nil
}

func readFileWithED25519PrivateKey(name string) (ed25519.PrivateKey, error) {
	privateKeyPemBytes, err := os.ReadFile(name)
	if err != nil {
		return ed25519.PrivateKey{}, err
	}
	privateKeyPemBlock, _ := pem.Decode(privateKeyPemBytes)
	if privateKeyPemBlock == nil {
		return ed25519.PrivateKey{}, err
	}
	privateKeyX509Bytes := privateKeyPemBlock.Bytes
	privateKeyAny, err := x509.ParsePKCS8PrivateKey(privateKeyX509Bytes)
	if err != nil {
		return ed25519.PrivateKey{}, err
	}
	privateKey, ok := privateKeyAny.(ed25519.PrivateKey)
	if !ok {
		return ed25519.PrivateKey{}, errors.New("not an ed25519 private key file")
	}
	return privateKey, nil
}

func readFileWithED25519PublicKey(name string) (ed25519.PublicKey, error) {
	publicKeyPemBytes, err := os.ReadFile(name)
	if err != nil {
		return ed25519.PublicKey{}, err
	}
	publicKeyPemBlock, _ := pem.Decode(publicKeyPemBytes)
	if publicKeyPemBlock == nil {
		return ed25519.PublicKey{}, err
	}
	publicKeyX509Bytes := publicKeyPemBlock.Bytes
	publicKeyAny, err := x509.ParsePKIXPublicKey(publicKeyX509Bytes)
	if err != nil {
		return ed25519.PublicKey{}, err
	}
	publicKey, ok := publicKeyAny.(ed25519.PublicKey)
	if !ok {
		return ed25519.PublicKey{}, errors.New("not an ed25519 public key file")
	}
	return publicKey, nil
}

var templateFuncs = template.FuncMap{
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

func newServer(db *pgxpool.Pool, mq *amqp091.Connection, s3Client *s3.Client, conf *Config) *http.Server {
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
	mux.HandleFunc("POST /build-div/build-create-form", func(w http.ResponseWriter, r *http.Request) {
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
		// If time_location is "", the server uses the default.
		const formValueTimeLocation = "time_location"
		timeLocationString := ""
		if part, err := peekPart(); err == nil && part.FormName() == formValueTimeLocation {
			_, _ = nextPart()
			timeLocationBytes, err := io.ReadAll(part)
			if err != nil {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
			timeLocationString = string(timeLocationBytes)
		}
		timeLocation, err := time.LoadLocation(timeLocationString)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}

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

		operationCreator := &build.Creator{DB: db, MQ: mq, S3: s3Client, BuildsAllowed: 10}
		operation, err := operationCreator.Create(r.Context(), &build.CreatorCreateParams{
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
		err = writeTemplate(w, "buildDiv", struct {
			Operation    *build.Build
			TimeLocation *time.Location
		}{
			Operation:    operation,
			TimeLocation: timeLocation,
		}, "main.tmpl")
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			err = writeErrorPage(w, http.StatusInternalServerError)
			if err != nil {
				slog.Error("failed to write error page", "err", err)
			}
		}
	})
	mux.HandleFunc("POST /header/sign-in", func(w http.ResponseWriter, r *http.Request) {
		privateKeyPemFile := ".run/jwt/private.pem"
		privateKeyPemBytes, err := os.ReadFile(privateKeyPemFile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		privateKeyPemBlock, _ := pem.Decode(privateKeyPemBytes)
		if privateKeyPemBlock == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		privateKeyX509Bytes := privateKeyPemBlock.Bytes
		privateKeyAny, err := x509.ParsePKCS8PrivateKey(privateKeyX509Bytes)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		privateKey, ok := privateKeyAny.(ed25519.PrivateKey)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		userID := uuid.New()

		// Cookie access_token.
		const cookieAccessToken = "access_token"
		accessTokenJWT := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(14 * 24 * time.Hour)), // TODO: Consider time zones.
			IssuedAt:  jwt.NewNumericDate(time.Now()),                          // TODO: Consider time zones.
			ID:        uuid.NewString(),
		})
		accessToken, err := accessTokenJWT.SignedString(privateKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		accessTokenCookie := &http.Cookie{
			Name:     cookieAccessToken,
			Value:    accessToken,
			Path:     "/",
			Domain:   "localhost",
			MaxAge:   int(14 * 24 * time.Hour / time.Second),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, accessTokenCookie)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		err = writeTemplate(w, "header", struct {
			User *struct{ ID uuid.UUID }
		}{
			User: &struct{ ID uuid.UUID }{ID: userID},
		}, "main.tmpl")
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			err = writeErrorPage(w, http.StatusInternalServerError)
			if err != nil {
				slog.Error("failed to write error page", "err", err)
			}
		}
	})
	mux.HandleFunc("POST /header/sign-out", func(w http.ResponseWriter, r *http.Request) {
		publicKeyPemFile := ".run/jwt/public.pem"
		publicKeyPemBytes, err := os.ReadFile(publicKeyPemFile)
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		publicKeyPemBlock, _ := pem.Decode(publicKeyPemBytes)
		if publicKeyPemBlock == nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		publicKeyX509Bytes := publicKeyPemBlock.Bytes
		publicKeyAny, err := x509.ParsePKIXPublicKey(publicKeyX509Bytes)
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		publicKey, ok := publicKeyAny.(ed25519.PublicKey)
		if !ok {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Cookie access_token.
		const cookieAccessToken = "access_token"
		accessTokenCookie, err := r.Cookie(cookieAccessToken)
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		err = accessTokenCookie.Valid()
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		accessToken := accessTokenCookie.Value
		accessTokenJWT, err := jwt.ParseWithClaims(
			accessToken,
			&jwt.RegisteredClaims{},
			func(t *jwt.Token) (interface{}, error) {
				return publicKey, nil
			},
			jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		)
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !accessTokenJWT.Valid { // TODO: Remove as it is likely redundant.
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		claims, ok := accessTokenJWT.Claims.(*jwt.RegisteredClaims) // TODO: Consider panicking instead as it is likely only influenced on what we pass to ParseWithClaims.
		if !ok {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		accessTokenExpiresAt := claims.ExpiresAt.Time // TODO: Check if we get time.Time correctly.
		accessTokenID, err := uuid.Parse(claims.ID)
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		{
			query := `
				SELECT EXISTS(
					SELECT 1
					FROM revoked_access_tokens
					WHERE id = $1
				)
			`
			args := []any{accessTokenID}

			rows, _ := db.Query(r.Context(), query, args...)
			exists, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[bool])
			if err != nil {
				slog.Error("", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if exists {
				slog.Error("", "err", err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		{
			query := `
				INSERT INTO revoked_access_tokens (id, expires_at)
				VALUES ($1, $2)
				ON CONFLICT (id) DO NOTHING
			`
			args := []any{accessTokenID, accessTokenExpiresAt}

			_, err := db.Exec(r.Context(), query, args...) // TODO: Check if ignoring the command tag is OK.
			if err != nil {
				slog.Error("", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		// Cookie access_token.
		const responseCookieAccessToken = "access_token"
		accessTokenResponseCookie := &http.Cookie{ // TODO: Check if all we need is Name and MaxAge.
			Name:     responseCookieAccessToken,
			Value:    "",
			Path:     "/",
			Domain:   "localhost",
			MaxAge:   -1, // Negative MaxAge causes deletes the cookie.
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, accessTokenResponseCookie)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		err = writeTemplate(w, "header", struct {
			User *struct{ ID uuid.UUID }
		}{
			User: nil,
		}, "main.tmpl")
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			err = writeErrorPage(w, http.StatusInternalServerError)
			if err != nil {
				slog.Error("failed to write error page", "err", err)
			}
		}
	})
	mux.HandleFunc("GET /build-div", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		// Cookie access_token.
		// ...
		userID := uuid.MustParse("3b4ff7e0-c540-4665-b148-af529d2f5be7")

		// Form value operation_id.
		const formValueOperationID = "operation_id"
		operationIDString := r.FormValue(formValueOperationID)
		operationID, err := uuid.Parse(operationIDString)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		// Form value time_location.
		// If time_location is "", the server uses the default.
		const formValueTimeLocation = "time_location"
		timeLocationString := r.FormValue(formValueTimeLocation)
		timeLocation, err := time.LoadLocation(timeLocationString)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		operationGetter := &build.Getter{DB: db}
		operation, err := operationGetter.Get(r.Context(), &build.GetterGetParams{
			ID:     operationID,
			UserID: userID,
		})
		if err != nil {
			slog.Error("", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		err = writeTemplate(w, "buildDiv", struct {
			Operation    *build.Build
			TimeLocation *time.Location
		}{
			Operation:    operation,
			TimeLocation: timeLocation,
		}, "main.tmpl")
		if err != nil {
			slog.Error("", "err", err)
			return
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
