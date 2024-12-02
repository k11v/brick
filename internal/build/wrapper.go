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
)

type RunWrapperParams struct {
	InputFiles  map[string][]byte
	OutputFiles map[string]struct{}
}

type RunWrapperResult struct {
	OutputFiles map[string][]byte
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
			var params RunWrapperParams
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
			err = func() error {
				name := p.Header.Get("X-Name")
				if name == "" {
					return fmt.Errorf("empty X-Name")
				}

				f, openErr := os.Open(name)
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
		Baz: "string",
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

	for i := 0; i < 3; i++ {
		partHeader = make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", "application/octet-stream")
		partBody, err = mw.CreatePart(partHeader)
		if err != nil {
			return fmt.Errorf("run wrapper: %w", err)
		}
		_, err = partBody.Write([]byte("file content"))
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
