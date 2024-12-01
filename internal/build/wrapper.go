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

type RunWrapperResult struct{}

// TODO: Consider accepting *bufio.Reader and *bufio.Writer.
func RunWrapper(in io.Reader, out io.Writer) error {
	pr := textproto.NewReader(bufio.NewReader(in))
	header, err := pr.ReadMIMEHeader()
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	contentType := header.Get("Content-Type")
	_, mediaTypeParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("run: %w", err)
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
			return fmt.Errorf("run: %w", err)
		}

		if partIndex == 1 {
			var params RunWrapperParams
			dec := json.NewDecoder(p)
			dec.DisallowUnknownFields()
			if err = dec.Decode(&params); err != nil {
				return fmt.Errorf("invalid request body: %w", err)
			}
			if dec.More() {
				return fmt.Errorf("invalid request body: multiple top-level values")
			}

			fmt.Printf("First part: %v\n", params)
		} else {
			var slurp []byte
			slurp, err = io.ReadAll(p)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}

			fmt.Printf("Subsequent part %d: %s\n", partIndex, slurp)
		}

		partIndex++
	}

	return nil
}
