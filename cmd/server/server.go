package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/amqputil"
)

func NewServer(db *pgxpool.Pool, mq *amqputil.Client, s3Client *s3.Client, conf *Config) (*http.Server, error) {
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

	mux.HandleFunc("GET /{$}", h.Page)
	mux.HandleFunc("POST /build_filesChangeToFiles", h.FilesChangeToFiles)
	mux.HandleFunc("POST /build_buildButtonClickToMain", h.BuildButtonClickToMain)
	mux.HandleFunc("GET /static/", h.StaticFile)
	mux.HandleFunc("GET /", h.NotFoundPage)

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
