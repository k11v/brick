package main

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
)

type fileWithoutData struct {
	Name string
	Type string
}

type ExecuteDocumentParams struct {
	DirEntries []*DirEntry
}

type DirEntry struct {
	Name       string
	Type       string
	DirEntries []*DirEntry
}

func (h *Handler) FilesChangeToFiles(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		h.serveError(w, r, fmt.Errorf("request: %w", err))
		return
	}

	var (
		bufPart  *multipart.Part
		bufErr   error
		bufNext  = false
		bufEmpty = true
	)
	nextPart := func() (*multipart.Part, error) {
		if bufNext {
			bufNext = false
			return bufPart, bufErr
		}
		bufPart, bufErr = mr.NextPart()
		bufEmpty = false
		return bufPart, bufErr
	}
	unnextPart := func() error {
		if bufNext || bufEmpty {
			return errors.New("unnextPart buf already next or empty")
		}
		bufNext = true
		return nil
	}

	files := make([]*fileWithoutData, 0)

FileLoop:
	for i := 0; ; i++ {
		var (
			name string
			typ  string
		)

	PartLoop:
		for {
			part, err := nextPart()
			if err != nil {
				if err == io.EOF {
					break FileLoop
				}
				h.serveError(w, r, fmt.Errorf("body: %w", err))
				return
			}

			formName := part.FormName()
			switch {
			// Form value files/*/name.
			case formName == fmt.Sprintf("files/%d/name", i):
				valueBytes, err := io.ReadAll(part)
				if err != nil {
					h.serveError(w, r, fmt.Errorf("%s form value: %w", formName, err))
					return
				}
				name = string(valueBytes)
			// Form value files/*/type.
			case formName == fmt.Sprintf("files/%d/type", i):
				valueBytes, err := io.ReadAll(part)
				if err != nil {
					h.serveError(w, r, fmt.Errorf("%s form value: %w", formName, err))
					return
				}
				typ = string(valueBytes)
			case strings.HasPrefix(formName, fmt.Sprintf("files/%d/", i+1)):
				err := unnextPart()
				if err != nil {
					h.serveError(w, r, err)
					return
				}
				break PartLoop
			default:
				h.serveError(w, r, fmt.Errorf("%s form name unknown or misplaced", formName))
				return
			}
		}

		file := &fileWithoutData{
			Name: name,
			Type: typ,
		}
		files = append(files, file)
	}

	dirEntryFromName := make(map[string]*DirEntry)
	dirEntryFromName["/"] = &DirEntry{
		Name:       path.Base("/"),
		Type:       "directory",
		DirEntries: nil,
	}

	for i, file := range files {
		if file.Name == "" {
			formName := fmt.Sprintf("files/%d/name", i)
			h.serveError(w, r, fmt.Errorf("%s form value empty or missing", formName))
			return
		}
		name := path.Join("/", file.Name)

		if file.Type == "" {
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value empty or missing", formName))
			return
		}
		switch file.Type {
		case "file", "directory":
		default:
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value unknown", formName))
			return
		}
		typ := file.Type

		if dirEntryFromName[name] != nil {
			formName := fmt.Sprintf("files/%d/name", i)
			h.serveError(w, r, fmt.Errorf("%s form value already exists", formName))
			return
		}
		dirEntryFromName[name] = &DirEntry{
			Name:       path.Base(name),
			Type:       typ,
			DirEntries: nil,
		}

		parentName := path.Dir(name)
		if dirEntryFromName[parentName] == nil {
			formName := fmt.Sprintf("files/%d/name", i)
			h.serveError(w, r, fmt.Errorf("%s form value not found", formName))
			return
		}
		if dirEntryFromName[parentName].Type != "directory" {
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value not directory", formName))
			return
		}
		dirEntryFromName[parentName].DirEntries = append(
			dirEntryFromName[parentName].DirEntries,
			dirEntryFromName[name],
		)
	}

	comp, err := h.execute("build_document", &ExecuteDocumentParams{
		DirEntries: dirEntryFromName["/"].DirEntries,
	})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(comp)
}
