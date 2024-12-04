package build

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type RunParams struct {
	OutputDir string
}

type RunResult struct {
	PDFFile  string
	LogFile  string
	ExitCode int
}

func Run(params *RunParams) (*RunResult, error) {
	result := RunResult{ExitCode: -1}

	// Create log file for Pandoc and Latexmk.
	logFile := filepath.Join(params.OutputDir, "log")
	if err := os.MkdirAll(params.OutputDir, 0o777); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	openLogFile, err := os.Create(logFile)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	defer openLogFile.Close()
	result.LogFile = logFile

	// Create metadata file for Pandoc.
	metadataFile := filepath.Join(params.OutputDir, "pandoc-input", "metadata.yaml")
	if err = os.MkdirAll(filepath.Dir(metadataFile), 0o777); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	err = os.WriteFile(
		metadataFile,
		[]byte(`mainfont: "CMU Serif"
mainfontfallback:
    - "Latin Modern Roman:"
    - "FreeSerif:"
    - "NotoColorEmoji:mode=harf"
sansfont: "CMU Sans Serif"
sansfontfallback:
    - "Latin Modern Sans:"
    - "FreeSans:"
    - "NotoColorEmoji:mode=harf"
monofont: "CMU Typewriter Text"
monofontfallback:
    - "Latin Modern Mono:"
    - "FreeMono:"
    - "NotoColorEmoji:mode=harf"
`),
		0o666,
	)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}

	// Run Pandoc.
	texFile := filepath.Join(params.OutputDir, "pandoc-output", "main.tex")
	if err = os.MkdirAll(filepath.Dir(texFile), 0o777); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	if _, err = openLogFile.Write([]byte("$ pandoc\n")); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	pandoc := exec.Command(
		"pandoc",
		"--verbose",
		"--from",
		"gfm",
		"--to",
		"latex",
		"--output",
		texFile,
		"--standalone",
		"--metadata-file",
		metadataFile,
		"main.md",
	)
	pandoc.Stdout = openLogFile
	pandoc.Stderr = openLogFile
	if err = pandoc.Run(); err != nil {
		if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return &result, nil
		}
		return nil, fmt.Errorf("run: %w", err)
	}

	// Run Latexmk.
	pdfFile := filepath.Join(params.OutputDir, "latexmk-output", "main.pdf")
	if err = os.MkdirAll(filepath.Dir(pdfFile), 0o777); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	if _, err = openLogFile.Write([]byte("$ latexmk\n")); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	latexmk := exec.Command(
		"latexmk",
		"-lualatex",
		"-interaction=nonstopmode",
		"-halt-on-error",
		"-file-line-error",
		"-shell-escape", // has security implications
		"-output-directory="+filepath.Dir(pdfFile),
		texFile,
	)
	latexmk.Stdout = openLogFile
	latexmk.Stderr = openLogFile
	if err = latexmk.Run(); err != nil {
		if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return &result, nil
		}
		return nil, fmt.Errorf("run: %w", err)
	}
	result.PDFFile = pdfFile
	result.ExitCode = 0

	return &result, nil
}