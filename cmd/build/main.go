package main

import (
	"flag"
	"fmt"
	"io"
	"os"
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
			_, _ = fmt.Fprintf(os.Stderr, "error: missing %s flag\n", flagInputFile)
			return 2
		}
		if *inputFile != "main.md" { // [Run] can only build main.md for now.
			_, _ = fmt.Fprintf(os.Stderr, "error: %s flag is not %q\n", flagInputFile, "main.md")
			return 2
		}

		const flagOutputFile = "-o"
		if *outputFile == "" {
			_, _ = fmt.Fprintf(os.Stderr, "error: missing %s flag\n", flagOutputFile)
			return 2
		}

		const flagCacheDir = "-c"
		if *cacheDir == "" {
			_, _ = fmt.Fprintf(os.Stderr, "error: missing %s flag\n", flagCacheDir)
			return 2
		}

		result, err := Run(&RunParams{
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
