package main

import (
	"archive/tar"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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

		// Read a tar file with input files from stdin
		// and extract it to the working directory.
		tr := tar.NewReader(os.Stdin)
		for {
			var h *tar.Header
			h, err := tr.Next()
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
				_, err = io.Copy(f, os.Stdin)
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

		// Run.
		result, err := build.Run(&build.RunParams{
			InputDir:  ".",
			OutputDir: *cacheDir,
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

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

		openResultLogFile, err := os.Open(result.LogFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer func() {
			_ = openResultLogFile.Close()
		}()
		_, err = io.Copy(os.Stdout, openResultLogFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		return result.ExitCode
	}
	os.Exit(run())
}
