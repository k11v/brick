package multifile

import (
	"errors"
	"io"
)

type ReadFunc func() (name string, content io.Reader, err error)

type Reader struct {
	readFunc ReadFunc
	name     string
	content  io.Reader
	err      error
}

func NewReader(readFunc ReadFunc) *Reader {
	return &Reader{readFunc: readFunc}
}

func (r *Reader) Read() bool {
	if r.err != nil {
		return false
	}
	r.name, r.content, r.err = r.readFunc()
	return r.err == nil
}

func (r *Reader) Name() string {
	return r.name
}

func (r *Reader) Content() io.Reader {
	if r.content == nil {
		panic("multifile: Content call before successful Read call")
	}
	return r.content
}

func (r *Reader) Err() error {
	if errors.Is(r.err, io.EOF) {
		return nil
	}
	return r.err
}
