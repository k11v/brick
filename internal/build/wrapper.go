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

	// Create log file for Pandoc and Latexmk.
	if err = os.MkdirAll(".brick", 0o777); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	logFile, err := os.Create(".brick/log")
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	// Create metadata file for Pandoc.
	if err = os.MkdirAll(".brick/pandoc-input", 0o777); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	metadataFile, err := os.Create(".brick/pandoc-input/metadata.yaml")
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	_, err = metadataFile.WriteString(`mainfont: "CMU Serif"
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
`)
	if err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	_ = metadataFile.Close() // TODO: defer

	// Create output directory for Pandoc.
	if err = os.MkdirAll(".brick/pandoc-output", 0o777); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	// Run Pandoc.
	if _, err = logFile.Write([]byte("$ pandoc\n")); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	pandoc := exec.Command(
		"pandoc",
		"--verbose",
		"--from",
		"gfm",
		"--to",
		"latex",
		"--output",
		".brick/pandoc-output/main.tex",
		"--standalone",
		"--metadata-file",
		".brick/pandoc-input/metadata.yaml",
		"main.md",
	)
	pandoc.Stdout = logFile
	pandoc.Stderr = logFile
	if err = pandoc.Run(); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	// Create output directory for Latexmk.
	if err = os.MkdirAll(".brick/latexmk-output", 0o777); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	// Run Latexmk.
	if _, err = logFile.Write([]byte("$ latexmk\n")); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}
	latexmk := exec.Command(
		"latexmk",
		"-lualatex",
		"-interaction=nonstopmode",
		"-halt-on-error",
		"-file-line-error",
		"-shell-escape", // has security implications
		"-output-directory=.brick/latexmk-output",
		".brick/pandoc-output/main.tex",
	)
	latexmk.Stdout = logFile
	latexmk.Stderr = logFile
	if err = latexmk.Run(); err != nil {
		return fmt.Errorf("run wrapper: %w", err)
	}

	_ = logFile.Close() // TODO: defer

	pdfPath := ".brick/latexmk-output/main.pdf"
	_ = pdfPath

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
