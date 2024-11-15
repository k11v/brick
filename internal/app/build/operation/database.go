package operation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/app/build"
)

var (
	ErrNotFound                  = errors.New("not found")
	ErrIdempotencyKeyAlreadyUsed = errors.New("idempotency key already used")
	ErrTxAlreadyClosed           = errors.New("tx already closed")
)

type Database interface {
	Begin(ctx context.Context) (DatabaseTx, error)
	LockBuilds(ctx context.Context, params *DatabaseLockBuildsParams) error
	GetBuildCount(ctx context.Context, params *DatabaseGetBuildCountParams) (int, error)
	CreateBuild(ctx context.Context, params *DatabaseCreateBuildParams) (*build.Build, error)
	GetBuild(ctx context.Context, params *DatabaseGetBuildParams) (*build.Build, error)
	GetBuildByIdempotencyKey(ctx context.Context, params *DatabaseGetBuildByIdempotencyKeyParams) (*build.Build, error)
	ListBuilds(ctx context.Context, params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error)
}

type DatabaseTx interface {
	Database

	// Commit returns a wrapped ErrTxAlreadyClosed if the transaction is already closed.
	// It is safe to call multiple times.
	Commit(ctx context.Context) error

	// Rollback returns a wrapped ErrTxAlreadyClosed if the transaction is already closed.
	// It is safe to call multiple times.
	Rollback(ctx context.Context) error
}

type DatabaseLockBuildsParams struct {
	UserID uuid.UUID
}

type DatabaseUnlockFunc func() error

type DatabaseGetBuildCountParams struct {
	EndTime   time.Time
	StartTime time.Time
	UserID    uuid.UUID
}

type DatabaseGetBuildCountResult struct {
	Count int
}

type DatabaseCreateBuildParams struct {
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
	DocumentToken  string

	ContextToken  string            // deprecated
	DocumentFiles map[string][]byte // deprecated
}

type DatabaseGetBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

type DatabaseGetBuildByIdempotencyKeyParams struct {
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
}

type DatabaseListBuildsParams struct {
	PageLimit  int
	PageOffset int
	UserID     uuid.UUID
}

type DatabaseListBuildsResult struct {
	Builds         []*build.Build
	NextPageOffset *int // zero value (nil) means no more pages
	TotalSize      int
}
