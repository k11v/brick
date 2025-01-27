package main

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/k11v/brick/internal/build"
)

type TreeFile struct {
	BaseName string
	Type     build.FileType
	Children []*TreeFile
}

type ListFile struct {
	Name string
	Type build.FileType
}

func TreeFilesFromListFiles(listFiles []*ListFile) ([]*TreeFile, error) {
	treeFiles := make(map[string]*TreeFile)
	treeFiles["/"] = &TreeFile{
		BaseName: "/",
		Type:     build.FileTypeDirectory,
		Children: make([]*TreeFile, 0),
	}

	for _, listFile := range listFiles {
		name := listFile.Name
		dirName := path.Dir(name)
		if _, found := treeFiles[dirName]; !found {
			return nil, fmt.Errorf("%s not found", dirName)
		}
		if _, found := treeFiles[name]; found {
			return nil, fmt.Errorf("%s already exists", name)
		}

		treeFile := &TreeFile{
			BaseName: path.Base(listFile.Name),
			Type:     listFile.Type,
			Children: make([]*TreeFile, 0),
		}
		treeFiles[name] = treeFile
		treeFiles[dirName].Children = append(treeFiles[dirName].Children, treeFile)
	}

	return treeFiles["/"].Children, nil
}

type ExecuteFilesParams struct {
	TreeFiles []*TreeFile
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

	listFiles := make([]*ListFile, 0)

	for i := 0; ; i++ {
		var (
			name      string
			typString string
			ok        bool
		)

		for {
			part, err := nextPart()
			if err != nil {
				if err == io.EOF {
					break
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
				ok = true
			// Form value files/*/type.
			case formName == fmt.Sprintf("files/%d/type", i):
				valueBytes, err := io.ReadAll(part)
				if err != nil {
					h.serveError(w, r, fmt.Errorf("%s form value: %w", formName, err))
					return
				}
				typString = string(valueBytes)
				ok = true
			case strings.HasPrefix(formName, fmt.Sprintf("files/%d/", i+1)):
				err := unnextPart()
				if err != nil {
					h.serveError(w, r, err)
					return
				}
				break
			default:
				h.serveError(w, r, fmt.Errorf("%s form name unknown or misplaced", formName))
				return
			}
		}
		if !ok {
			break
		}

		name = path.Join("/", name)

		typ, known := build.ParseFileType(typString)
		if !known {
			formName := fmt.Sprintf("files/%d/type", i)
			h.serveError(w, r, fmt.Errorf("%s form value unknown", formName))
			return
		}

		listFiles = append(listFiles, &ListFile{
			Name: name,
			Type: typ,
		})
	}

	slices.SortFunc(listFiles, func(a, b *ListFile) int {
		return strings.Compare(a.Name, b.Name)
	})

	treeFiles, err := TreeFilesFromListFiles(listFiles)
	if err != nil {
		h.serveError(w, r, err)
		return
	}

	comp, err := h.execute("build_files", &ExecuteFilesParams{TreeFiles: treeFiles})
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(comp)
}
