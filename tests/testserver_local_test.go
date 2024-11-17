//go:build local

package tests

import (
	"context"
	"testing"
)

func NewTestServer(tb testing.TB, ctx context.Context) (baseURL string) {
	tb.Helper()

	return "http://127.0.0.1:8080"
}
