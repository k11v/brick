package main

import (
	"archive/tar"
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"

	"github.com/k11v/brick/internal/build"
)

var (
	inputFile  = flag.String("i", "", "Markdown input file")
	outputFile = flag.String("o", "", "PDF output file")
	cacheDir   = flag.String("c", "", "cache dir")
)

func main() {
	run := func() int {
		flag.Parse()

		const flagInputFile = "-i"
		if *inputFile == "" {
			_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagInputFile)
			return 2
		}
		if *inputFile != "main.md" { // build.Run can only build main.md for now.
			_, _ = fmt.Fprintf(os.Stderr, "error: %s flag is not %q\n", flagInputFile, "main.md")
			return 2
		}

		const flagOutputFile = "-o"
		if *outputFile == "" {
			_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagOutputFile)
			return 2
		}

		const flagCacheDir = "-c"
		if *cacheDir == "" {
			_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagCacheDir)
			return 2
		}

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

		// Read a tar file with input files from stdin
		// and extract it to the working directory.
		mr := multipart.NewReader(bufstdin, boundary)
		tarFile, err := mr.NextPart()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		tr := tar.NewReader(tarFile)
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
				f, err = os.Create(h.Name)
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
				err = os.Mkdir(h.Name, 0o777)
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, err.Error())
					return 1
				}
			default:
				_, _ = fmt.Fprintln(os.Stderr, "error: unsupported tar entry type")
				return 1
			}
		}
		_, err = mr.NextPart()
		if !errors.Is(err, io.EOF) {
			_, _ = fmt.Fprintln(os.Stderr, "error: extra stdin part")
			return 1
		}

		// Run.
		result, err := build.Run(&build.RunParams{
			InputDir:  ".",
			OutputDir: *cacheDir,
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		if result.ExitCode == 0 {
			openOutputFile, err := os.Create(*outputFile)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
			defer func() {
				_ = openOutputFile.Close()
			}()

			openResultPDFFile, err := os.Open(result.PDFFile)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
			defer func() {
				_ = openResultPDFFile.Close()
			}()
			_, err = io.Copy(openOutputFile, openResultPDFFile)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
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
