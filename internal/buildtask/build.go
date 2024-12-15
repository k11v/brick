package buildtask

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

// Status represents the build status as a string.
type Status string

const (
	// StatusPending indicates that the build is pending.
	StatusPending Status = "pending"
	// StatusRunning indicates that the build is currently running.
	StatusRunning Status = "running"
	// StatusCompleted indicates that the build has completed successfully.
	StatusCompleted Status = "completed"
	// StatusCanceled indicates that the build has been canceled.
	StatusCanceled Status = "canceled"
)

var statuses = map[Status]struct{}{
	StatusPending:   {},
	StatusRunning:   {},
	StatusCompleted: {},
	StatusCanceled:  {},
}

// StatusFromString converts a string to a Status type and checks if it is a known status.
// It returns the Status and a boolean indicating whether the status is known.
func StatusFromString(s string) (status Status, known bool) {
	status = Status(s)
	_, known = statuses[status]
	return status, known
}
