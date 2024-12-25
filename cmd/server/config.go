package main

import (
	"time"
)

type Config struct {
	JWTSignatureKeyFile    string // required
	JWTVerificationKeyFile string // required

	Host              string // default: "127.0.0.1"
	Port              int    // default: 8080
	ReadHeaderTimeout time.Duration
}

func (cfg *Config) host() string {
	h := cfg.Host
	if h == "" {
		h = "127.0.0.1"
	}
	return h
}

func (cfg *Config) port() int {
	p := cfg.Port
	if p == 0 {
		p = 8080
	}
	return p
}
