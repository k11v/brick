package build

import (
	"time"
)

type Builder struct{}

type BuildParams struct {
	Files       []string
	InputFiles  map[string][]byte
	OutputFiles map[string]struct{}

	// ID                  uuid.UUID
	// IdempotencyKey      uuid.UUID
	// InputDocumentStoragePrefix string
	// InputCacheStoragePrefix    string
	// OutputDocumentStorageKey   string
	// OutputCacheStoragePrefix   string
	// ProcessLogStorageKey       string
}

type BuildResult struct {
	ProcessUsedTime   time.Duration
	ProcessUsedMemory int
	ProcessExitCode   int
}

func (*Builder) Build(params *BuildParams) (*BuildResult, error) {
	panic("unimplemented")
}
