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
)

type RunWrapperParams struct {
	Foo *string
	Bar *int
}

type RunWrapperResult struct {
	Baz string
}

// TODO: Consider accepting *bufio.Reader and *bufio.Writer.
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
			var slurp []byte
			slurp, err = io.ReadAll(p)
			if err != nil {
				return fmt.Errorf("run wrapper: %w", err)
			}

			fmt.Printf("Subsequent part %d: %s\n", partIndex, slurp)
		}

		partIndex++
	}

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
