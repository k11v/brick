package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/k11v/brick/internal/app"
)

func main() {
	ctx := context.Background()

	err := SetupJWT(&SetupJWTParams{
		PublicKeyFile:  ".run/jwt/public.pem",
		PrivateKeyFile: ".run/jwt/private.pem",
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = app.SetupPostgres("postgres://postgres:postgres@127.0.0.1:5432/postgres")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = app.SetupS3(ctx, "http://minioadmin:minioadmin@127.0.0.1:9000")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

type SetupJWTParams struct {
	PublicKeyFile  string
	PrivateKeyFile string
}

func SetupJWT(params *SetupJWTParams) error {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}

	pubX509, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	pubPem := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubX509,
	}
	pubPemBuf := new(bytes.Buffer)
	err = pem.Encode(pubPemBuf, pubPem)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(params.PublicKeyFile), 0o777)
	if err != nil {
		return err
	}
	err = os.WriteFile(params.PublicKeyFile, pubPemBuf.Bytes(), 0o666) // TODO: Consider less permissions.
	if err != nil {
		return err
	}

	privX509, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privPem := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privX509,
	}
	privPemBuf := new(bytes.Buffer)
	err = pem.Encode(privPemBuf, privPem)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(params.PrivateKeyFile), 0o777)
	if err != nil {
		return err
	}
	err = os.WriteFile(params.PrivateKeyFile, privPemBuf.Bytes(), 0o666) // TODO: Consider less permissions.
	if err != nil {
		return err
	}

	return nil
}
