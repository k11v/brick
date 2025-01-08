package main

import (
	"archive/tar"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/k11v/brick/internal/build"
)

type Params struct {
	InputFile string `json:"input_file"`
}

func main() {
	run := func() int {
		// Peek stdin and detect the multipart boundary.
		// The boundary line should be less than 74 bytes:
		// 2 bytes for "--", up to 70 bytes for user-defined boundary, and 2 bytes for "\r\n".
		// See https://datatracker.ietf.org/doc/html/rfc1341.
		bufstdin := bufio.NewReader(os.Stdin)
		peek, err := bufstdin.Peek(74)
		if err != nil && err != io.EOF {
			_, _ = fmt.Fprintf(os.Stderr, "error: invalid stdin boundary: %v\n", err)
			return 2
		}
		boundary := string(peek)
		if boundary[:2] != "--" {
			_, _ = fmt.Fprint(os.Stderr, "error: invalid stdin boundary start\n")
			return 2
		}
		boundaryEnd := strings.Index(boundary, "\r\n")
		if boundaryEnd == -1 {
			_, _ = fmt.Fprint(os.Stderr, "error: invalid stdin boundary length or end\n")
			return 2
		}
		boundary = boundary[2:boundaryEnd]

		mr := multipart.NewReader(bufstdin, boundary)

		// Read tar with input files from stdin and
		// extract it to the input directory.
		err = os.Mkdir("input", 0o777)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		tarContent, err := mr.NextPart()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		tr := tar.NewReader(tarContent)
		for {
			var h *tar.Header
			h, err = tr.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				_, _ = fmt.Fprintln(os.Stderr, err.Error())
				return 1
			}
			switch h.Typeflag {
			case tar.TypeReg:
				var f *os.File
				f, err = os.Create(filepath.Join("input", h.Name))
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, err.Error())
					return 1
				}
				_, err = io.Copy(f, tr)
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, err.Error())
					return 1
				}
				_ = f.Close()
			case tar.TypeDir:
				err = os.Mkdir(filepath.Join("input", h.Name), 0o777)
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, err.Error())
					return 1
				}
			default:
				_, _ = fmt.Fprintln(os.Stderr, "error: unsupported tar entry type")
				return 1
			}
		}

		// Read and decode JSON with params from stdin.
		var params Params
		paramsContent, err := mr.NextPart()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			return 2
		}
		dec := json.NewDecoder(paramsContent)
		dec.DisallowUnknownFields()
		err = dec.Decode(&params)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		if dec.More() {
			_, _ = fmt.Fprint(os.Stderr, "error: multiple top-level values\n")
			return 2
		}
		if params.InputFile == "" {
			_, _ = fmt.Fprintf(os.Stderr, "error: empty %s param\n", "input_file")
			return 2
		}
		if params.InputFile != "main.md" { // build.Run can only build main.md for now.
			_, _ = fmt.Fprintf(os.Stderr, "error: %s param is not %q\n", "input_file", "main.md")
			return 2
		}

		// Check stdin doesn't have extra parts.
		_, err = mr.NextPart()
		if !errors.Is(err, io.EOF) {
			_, _ = fmt.Fprintln(os.Stderr, "error: extra stdin part")
			return 2
		}

		// Run.
		result, err := build.Run(&build.RunParams{
			InputDir:  "input",
			OutputDir: "output",
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		mw := multipart.NewWriter(os.Stdout)
		defer func() {
			_ = mw.Close()
		}()

		logFileOut, err := mw.CreatePart(textproto.MIMEHeader{})
		if err != nil {
			panic(err)
		}
		openResultLogFile, err := os.Open(result.LogFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer func() {
			_ = openResultLogFile.Close()
		}()
		_, err = io.Copy(logFileOut, openResultLogFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		outputFileOut, err := mw.CreatePart(textproto.MIMEHeader{})
		if err != nil {
			panic(err)
		}
		openResultOutputFile, err := os.Open(result.PDFFile)
		if err != nil {
			panic(err)
		}
		defer func() {
			_ = openResultOutputFile.Close()
		}()
		_, err = io.Copy(outputFileOut, openResultOutputFile)
		if err != nil {
			panic(err)
		}

		return result.ExitCode
	}
	os.Exit(run())
}
