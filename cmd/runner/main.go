package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"os"
)

var boundary = flag.String("boundary", "", "set the boundary of multipart content (required)")

func main() {
	flag.Parse()
	if *boundary != "" {
		_, _ = fmt.Fprintln(os.Stderr, "empty -boundary flag")
		os.Exit(2)
	}

	mr := multipart.NewReader(os.Stdin, *boundary)
	inputTar, err := mr.NextPart()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	_, err = mr.NextPart()
	if !errors.Is(err, io.EOF) {
		_, _ = fmt.Fprintln(os.Stderr, "extra stdin part")
		os.Exit(2)
	}
	_ = inputTar
}
