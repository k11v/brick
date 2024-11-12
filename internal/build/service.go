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
