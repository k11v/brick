package build

import (
	"bytes"
	"testing"
)

func TestWrapper(t *testing.T) {
	t.Run("runs", func(t *testing.T) {
		in := bytes.NewReader([]byte("Content-Type: multipart/form-data; boundary=foo\r\n" +
			"\r\n" +
			"--foo\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n" +
			`{"foo": "string", "bar": 42}` + "\r\n" +
			"--foo--\r\n"))
		out := new(bytes.Buffer) // e.g. "Content-Type: multipart/form-data; boundary=1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\n\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: applcation/json\r\n\r\n{\"Baz\":\"string\"}\n\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: application/octet-stream\r\n\r\nfile content\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: application/octet-stream\r\n\r\nfile content\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: application/octet-stream\r\n\r\nfile content\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962--\r\n"

		if err := RunWrapper(in, out); err != nil {
			t.Fatalf("didn't want %q", err)
		}
	})
}
