package build

import (
	"time"
)

type BuildParams struct {
	// ID                  uuid.UUID
	// IdempotencyKey      uuid.UUID
	InputDocumentStoragePrefix string
	InputCacheStoragePrefix    string
	OutputDocumentStorageKey   string
	OutputCacheStoragePrefix   string
	ProcessLogStorageKey       string
}

type BuildResult struct {
	ProcessUsedTime   time.Duration
	ProcessUsedMemory int
	ProcessExitCode   int
}

func Build(params BuildParams) (*BuildResult, error) {
	panic("unimplemented")
}
