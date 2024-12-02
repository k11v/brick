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
