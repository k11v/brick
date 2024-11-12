package build

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrLimitExceeded             = errors.New("limit exceeded")
	ErrIdempotencyKeyAlreadyUsed = errors.New("idempotency key already used")
	ErrDatabaseNotFound          = errors.New("database: not found")
)

type Status int

const (
	StatusCreated Status = iota
	StatusRunning
	StatusCompleted
	StatusCanceled
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
}

type Database interface {
	BeginFunc(ctx context.Context, f func(tx Database) error) error
	LockUser(ctx context.Context, params *DatabaseLockUserParams) error
	GetBuildCount(ctx context.Context, params *DatabaseGetBuildCountParams) (int, error)
	CreateBuild(ctx context.Context, params *DatabaseCreateBuildParams) (*DatabaseBuild, error)
	GetBuild(ctx context.Context, params *DatabaseGetBuildParams) (*DatabaseBuild, error)
	GetBuildByIdempotencyKey(ctx context.Context, params *DatabaseGetBuildByIdempotencyKeyParams) (*DatabaseBuild, error)
	ListBuilds(ctx context.Context, params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error)
}

type DatabaseBuild struct {
	Done             bool
	Error            error
	ID               uuid.UUID
	NextContextToken string
	OutputFile       []byte
}

type DatabaseLockUserParams struct {
	UserID uuid.UUID
}

type DatabaseGetBuildCountParams struct {
	EndTime   time.Time
	StartTime time.Time
	UserID    uuid.UUID
}

type DatabaseGetBuildCountResult struct {
	Count int
}

type DatabaseCreateBuildParams struct {
	ContextToken   string
	DocumentFiles  map[string][]byte
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
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
	Builds         []*DatabaseBuild
	NextPageOffset *int // zero value (nil) means no more pages
	TotalSize      int
}

type Storage interface{}

type Broker interface{}

type Config struct {
	BuildsAllowed int
}

type Service struct {
	config   *Config  // required
	database Database // required
	storage  Storage  // required
	broker   Broker   // required
}

func NewService(config *Config, database Database, storage Storage, broker Broker) Service {
	return Service{config: config, database: database, storage: storage, broker: broker}
}

type CreateBuildParams struct {
	ContextToken   string
	DocumentFiles  map[string][]byte
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
}

func (s *Service) CreateBuild(ctx context.Context, createBuildParams *CreateBuildParams) (*Build, error) {
	var b *Build

	databaseBuildByIdempotencyKey, err := s.database.GetBuildByIdempotencyKey(ctx, &DatabaseGetBuildByIdempotencyKeyParams{
		IdempotencyKey: createBuildParams.IdempotencyKey,
		UserID:         createBuildParams.UserID,
	})
	if err != nil && !errors.Is(err, ErrDatabaseNotFound) {
		// TODO: Handle access denied.
		return nil, err
	}
	if err == nil {
		// FIXME: Check request payload.
		b = buildFromDatabaseBuild(databaseBuildByIdempotencyKey)
		return b, nil
	}
	// TODO: Consider a race condition where multiple requests determine
	// that an idempotency key can be used and fail later
	// when Database.CreateBuild is called.

	err = s.database.BeginFunc(ctx, func(tx Database) error {
		if err := tx.LockUser(ctx, &DatabaseLockUserParams{UserID: createBuildParams.UserID}); err != nil {
			return err
		}

		startTime := time.Now().UTC().Truncate(24 * time.Hour)
		endTime := startTime.Add(24 * time.Hour)

		used, err := tx.GetBuildCount(ctx, &DatabaseGetBuildCountParams{
			UserID:    createBuildParams.UserID,
			StartTime: startTime,
			EndTime:   endTime,
		})
		if err != nil {
			return err
		}

		if used >= s.config.BuildsAllowed {
			return ErrLimitExceeded
		}

		databaseBuild, err := tx.CreateBuild(ctx, &DatabaseCreateBuildParams{
			ContextToken:   createBuildParams.ContextToken,
			DocumentFiles:  createBuildParams.DocumentFiles,
			IdempotencyKey: createBuildParams.IdempotencyKey,
			UserID:         createBuildParams.UserID,
		})
		if err != nil {
			return err
		}

		b = buildFromDatabaseBuild(databaseBuild)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

type GetBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (s *Service) GetBuild(ctx context.Context, getBuildParams *GetBuildParams) (*Build, error) {
	panic("not implemented")
}

// TODO: maybe use context.
type GetBuildWithTimeout struct {
	ID      uuid.UUID
	Timeout time.Duration
	UserID  uuid.UUID
}

func (s *Service) GetBuildWithTimeout(ctx context.Context, getBuildWithTimeoutParams *GetBuildWithTimeout) (*Build, error) {
	panic("not implemented")
}

type ListBuildsParams struct {
	Filter    string
	PageSize  int    // zero value (0) means default, constrained, passed to LIMIT
	PageToken string // parsed as int, passed to OFFSET
	UserID    uuid.UUID
}

type ListBuildsResult struct {
	Builds        []*Build
	NextPageToken string // zero value ("") means no more pages
	TotalSize     int
}

func (s *Service) ListBuilds(ctx context.Context, listBuildsParams *ListBuildsParams) (*ListBuildsResult, error) {
	panic("not implemented")
}

type CancelBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// CancelBuild.
// It is idempotent without idempotency key.
func (s *Service) CancelBuild(ctx context.Context, cancelBuildParams *CancelBuildParams) error {
	panic("not implemented")
}

type GetLimitsParams struct {
	UserID uuid.UUID
}

type GetLimitsResult struct {
	BuildsAllowed int
	BuildsUsed    int
	ResetsAt      time.Time
}

func (s *Service) GetLimits(ctx context.Context, getLimitsParams *GetLimitsParams) (*GetLimitsResult, error) {
	panic("not implemented")
}

func buildFromDatabaseBuild(databaseBuild *DatabaseBuild) *Build {
	return &Build{
		// Done:             databaseBuild.Done,
		// Error:            databaseBuild.Error,
		ID: databaseBuild.ID,
		// NextContextToken: databaseBuild.NextContextToken,
		OutputFile: databaseBuild.OutputFile,
	}
}
