package main

import (
	"archive/tar"
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
		os.Exit(1)
	}

	mr := multipart.NewReader(os.Stdin, *boundary)

	tarPart, err := mr.NextPart()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	tr := tar.NewReader(tarPart)
	for {
		var h *tar.Header
		h, err = tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		switch h.Typeflag {
		case tar.TypeReg:
			var f *os.File
			f, err = os.Create(h.Name)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
			_, err = io.Copy(f, tarPart)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
			_ = f.Close()
		case tar.TypeDir:
			err = os.Mkdir(h.Name, 0o777)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
		default:
			_, _ = fmt.Fprintln(os.Stderr, "unsupported tar entry type")
			os.Exit(1)
		}
	}

	_, err = mr.NextPart()
	if !errors.Is(err, io.EOF) {
		_, _ = fmt.Fprintln(os.Stderr, "extra stdin part")
		os.Exit(1)
	}
}
