package build

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type RunWrapperParams struct {
	InputFiles  map[string][]byte
	OutputFiles map[string]struct{}
}

type RunWrapperResult struct {
	OutputFiles map[string][]byte
	LogFile     []byte
	UsedTime    time.Duration
	UsedMemory  int
	ExitCode    int
}

// TODO: Consider accepting *bufio.Reader and *bufio.Writer.
//
// TODO: Maybe switch textproto to net/http.ReadRequest for simpler code,
// easier composability and testability (e.g. with CLI tools like HTTPie
// and packages like net/http/httptest).
func RunWrapper(in io.Reader, out io.Writer) error {
	pr := textproto.NewReader(bufio.NewReader(in))
	header, err := pr.ReadMIMEHeader()
	if errors.Is(err, io.EOF) {
		// continue
	} else if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	contentType := header.Get("Content-Type")
	_, mediaTypeParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	boundary := mediaTypeParams["boundary"]

	var params RunWrapperParams
	mr := multipart.NewReader(pr.R, boundary)
	partIndex := 0
	for {
		var p *multipart.Part
		p, err = mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("run wrapper: %w", err)
		}

		if partIndex == 0 {
			dec := json.NewDecoder(p)
			dec.DisallowUnknownFields()
			if err = dec.Decode(&params); err != nil {
				return fmt.Errorf("run wrapper: %w", err)
			}
			if dec.More() {
				return fmt.Errorf("run wrapper: multiple top-level values")
			}

			fmt.Printf("First part: %v\n", params)
		} else {
			// Handle subsequent parts.
			// Name could be absolute, but we are fine with that here.
			// TODO: Could third-parties interfer with X-Name?
			// TODO: Consider X-Force that truncates a file if it exists instead of erroring out.
			// I don't see much use in that for now.
			// TODO: Should I trust umask to change standard 0666 to 0644 or 0600, or should I just create it this way?
			// TODO: Test a case where file that wrapper tries to create exists.
			err = func() error {
				name := p.Header.Get("X-Name")
				if name == "" {
					return fmt.Errorf("empty X-Name")
				}

				f, openErr := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
				if openErr != nil {
					return openErr
				}
				defer f.Close()

				if _, copyErr := io.Copy(f, p); copyErr != nil {
					return copyErr
				}
				return nil
			}()
			if err != nil {
				return fmt.Errorf("run wrapper: %w", err)
			}
		}

		partIndex++
	}

	// Flow:
	// 1. Write input files
	// 2. Run program
	// 3. Read output files
	//
	// Question: How to pass filepaths?
	//
	// Question: Is it correct to use form-data?
	// Does it has any implications?
	// It feels like I'd prefer mixed.
	//
	// Question: Can I rely on the first part to always be
	// the main JSON payload when using chosen content type?

	runResult, err := Run(&RunParams{InternalOutputDir: ".brick"})
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	_ = runResult

	result := RunWrapperResult{
		OutputFiles: map[string][]byte{},
		LogFile:     []byte{},
		UsedTime:    0,
		UsedMemory:  0,
		ExitCode:    0,
	}

	pw := textproto.NewWriter(bufio.NewWriter(out))
	mw := multipart.NewWriter(pw.W)

	_, err = pw.W.Write([]byte("Content-Type: " + mw.FormDataContentType() + "\r\n\r\n"))
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Type", "applcation/json")
	partBody, err := mw.CreatePart(partHeader)
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	if err = json.NewEncoder(partBody).Encode(result); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	for fileName := range params.OutputFiles {
		partHeader = make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", "application/octet-stream")
		partHeader.Set("X-Name", fileName) // FIXME: X-Name header _value_ should be escaped or encoded.
		partBody, err = mw.CreatePart(partHeader)
		if err != nil {
			return fmt.Errorf("run wrapper: %w", err)
		}

		// Errors like "Is a directory", "Permission denied", "File doesn't exist"
		// should probably be reported. For now we fail the whole process.
		//
		// Also output files should really be output paths and support directories.
		// E.g. I probably want to write and read the entire cache directory
		// because I don't know exact file names that will be generated.
		err = func() error {
			f, err := os.Open(fileName)
			if err != nil {
				// no file is still a result, i think
				return fmt.Errorf("run wrapper: %w", err)
			}
			defer f.Close()

			// TODO: How can reading/writing from/to an already open file fail?
			_, err = io.Copy(partBody, f)
			if err != nil {
				return fmt.Errorf("run wrapper: %w", err)
			}
			return nil
		}()
		if err != nil {
			return fmt.Errorf("run wrapper: %w", err)
		}
	}

	if err = mw.Close(); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	if err = pw.W.Flush(); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	return nil
}

type RunParams struct {
	InternalOutputDir string
}

type RunResult struct {
	PDFFile  string
	LogFile  string
	ExitCode int
}

func Run(params *RunParams) (*RunResult, error) {
	result := RunResult{ExitCode: -1}

	// Create log file for Pandoc and Latexmk.
	logFile := filepath.Join(params.InternalOutputDir, "log")
	if err := os.MkdirAll(params.InternalOutputDir, 0o777); err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
	}
	openLogFile, err := os.Create(logFile)
	if err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
	}
	defer openLogFile.Close()
	result.LogFile = logFile

	// Create metadata file for Pandoc.
	metadataFile := filepath.Join(params.InternalOutputDir, "pandoc-input", "metadata.yaml")
	if err = os.MkdirAll(filepath.Dir(metadataFile), 0o777); err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
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
		return nil, fmt.Errorf("run wrapper: %w", err)
	}

	// Run Pandoc.
	texFile := filepath.Join(params.InternalOutputDir, "pandoc-output", "main.tex")
	if err = os.MkdirAll(filepath.Dir(texFile), 0o777); err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
	}
	if _, err = openLogFile.Write([]byte("$ pandoc\n")); err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
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
		return nil, fmt.Errorf("run wrapper: %w", err)
	}

	// Run Latexmk.
	pdfFile := filepath.Join(params.InternalOutputDir, "latexmk-output", "main.pdf")
	if err = os.MkdirAll(filepath.Dir(pdfFile), 0o777); err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
	}
	if _, err = openLogFile.Write([]byte("$ latexmk\n")); err != nil {
		return nil, fmt.Errorf("run wrapper: %w", err)
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
		return nil, fmt.Errorf("run wrapper: %w", err)
	}
	result.PDFFile = pdfFile
	result.ExitCode = 0

	return &result, nil
}
