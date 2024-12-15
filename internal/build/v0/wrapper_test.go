package build

import (
	"bytes"
	"testing"
)

func TestHandleRun(t *testing.T) {
	t.Run("runs", func(t *testing.T) {
		t.Skip() // skipping for now because HandleRun creates files in the current working directory

		stdin := bytes.NewReader([]byte("Content-Type: multipart/form-data; boundary=foo\r\n" +
			"\r\n" +
			"--foo\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n" +
			`{"foo": "string", "bar": 42}` + "\r\n" +
			"--foo--\r\n"))
		stdout := new(bytes.Buffer) // e.g. "Content-Type: multipart/form-data; boundary=1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\n\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: applcation/json\r\n\r\n{\"Baz\":\"string\"}\n\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: application/octet-stream\r\n\r\nfile content\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: application/octet-stream\r\n\r\nfile content\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962\r\nContent-Type: application/octet-stream\r\n\r\nfile content\r\n--1751ed1b0d005fc0b53c86b5330ddc0fa4cb63cbf73a61c47df95aafb962--\r\n"

		if err := HandleRun(stdin, stdout); err != nil {
			t.Fatalf("didn't want %q", err)
		}
	})
}
