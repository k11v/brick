package build

import "io"

type File struct {
	Name string
	Data io.Reader
}
