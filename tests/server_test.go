package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestServer(t *testing.T) {
	t.Run(`GET /health returns OK`, func(t *testing.T) {
		ctx := context.Background()
		baseURL := NewTestServer(t, ctx)

		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		t.Cleanup(func() {
			_ = resp.Body.Close()
		})

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		bodyString := string(body)

		if got, want := resp.StatusCode, http.StatusOK; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if got, want := bodyString, `{"status":"ok"}`; !EqualJSON(got, want) {
			t.Logf("got %s", got)
			t.Fatalf("want %s", want)
		}
	})
}

func NewTestServer(tb testing.TB, ctx context.Context) (baseURL string) {
	tb.Helper()
	return "http://127.0.0.1:8080"
}

func EqualJSON(x, y string) bool {
	var mx, my any
	if err := json.Unmarshal([]byte(x), &mx); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(y), &my); err != nil {
		return false
	}
	return reflect.DeepEqual(mx, my)
}
