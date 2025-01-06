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

var (
	boundary   = flag.String("b", "", "stdin MIME multipart boundary")
	inputFile  = flag.String("i", "", "Markdown input file")
	outputFile = flag.String("o", "", "PDF output file")
	cacheDir   = flag.String("c", "", "cache dir")
)

func main() {
	flag.Parse()

	const flagBoundary = "-b"
	if *boundary != "" {
		_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagBoundary)
		os.Exit(2)
	}

	const flagInputFile = "-i"
	if *inputFile == "" {
		_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagInputFile)
		os.Exit(2)
	}
	if *inputFile != "main.md" { // build.Run can only build main.md for now.
		_, _ = fmt.Fprintf(os.Stderr, "error: %s flag is not %q\n", flagInputFile, "main.md")
		os.Exit(2)
	}

	const flagOutputFile = "-o"
	if *outputFile == "" {
		_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagOutputFile)
		os.Exit(2)
	}

	const flagCacheDir = "-c"
	if *cacheDir == "" {
		_, _ = fmt.Fprintf(os.Stderr, "error: empty %s flag\n", flagCacheDir)
		os.Exit(2)
	}

	// Read a tar file with input files from stdin
	// and extract it to the working directory.
	mr := multipart.NewReader(os.Stdin, *boundary)
	tarFile, err := mr.NextPart()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
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
			_, err = io.Copy(f, tarFile)
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
			_, _ = fmt.Fprintln(os.Stderr, "error: unsupported tar entry type")
			os.Exit(1)
		}
	}
	_, err = mr.NextPart()
	if !errors.Is(err, io.EOF) {
		_, _ = fmt.Fprintln(os.Stderr, "error: extra stdin part")
		os.Exit(1)
	}

	fmt.Println("done")

	os.Exit(0)
}
