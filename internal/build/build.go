package build

import (
	"time"

	"github.com/google/uuid"
)

type Build struct {
	ID             uuid.UUID
	IdempotencyKey uuid.UUID

	UserID    uuid.UUID
	CreatedAt time.Time

	DocumentToken string // instead of DocumentCacheFiles map[string][]byte
	DocumentFiles map[string][]byte

	ProcessLogFile    []byte
	ProcessUsedTime   time.Duration
	ProcessUsedMemory int
	ProcessExitCode   int

	OutputFile        []byte
	NextDocumentToken string // instead of OutputCacheFiles map[string][]byte
	OutputExpiresAt   time.Time

	Status Status
	Done   bool
}
