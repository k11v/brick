package build

import (
	"bytes"
	"testing"
)

func TestWrapper(t *testing.T) {
	t.Run("runs", func(t *testing.T) {
		in := bytes.NewReader([]byte("Content-Type: multipart/mixed; boundary=foo\r\n" +
			"\r\n" +
			"--foo\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n" +
			`{"foo": "string", "bar": 42}` + "\r\n" +
			"--foo--\r\n"))
		out := new(bytes.Buffer)

		if err := RunWrapper(in, out); err != nil {
			t.Fatalf("didn't want %q", err)
		}
	})
}
