package operation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/app/build"
)

var (
	ErrLimitExceeded    = errors.New("limit exceeded")
	ErrDatabaseNotFound = errors.New("database: not found")
)

type Service struct {
	config  *Config  // required
	db      Database // required
	storage Storage  // required
	broker  Broker   // required
}

func NewService(config *Config, database Database, storage Storage, broker Broker) Service {
	return Service{config: config, db: database, storage: storage, broker: broker}
}

type CreateBuildParams struct {
	ContextToken   string
	DocumentFiles  map[string][]byte
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
}

// CreateBuild.
//
// TODO: Check the params when a build is found by the idempotency key.
//
// FIXME: There is a race condition when two parallel Service.CreateBuild calls
// for the same idempotency key conclude that the key is unused, proceed
// to Database.CreateBuild and fail.
//
// FIXME: There is a problem when Database.GetBuildByIdempotencyKey
// doesn't get a build not because the idempotency key is unused
// but because the user is different.
func (s *Service) CreateBuild(ctx context.Context, params *CreateBuildParams) (*build.Build, error) {
	b, err := s.db.GetBuildByIdempotencyKey(ctx, &DatabaseGetBuildByIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKey,
		UserID:         params.UserID,
	})
	if err == nil {
		return b, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("service create build: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("service create build: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = tx.LockBuilds(ctx, &DatabaseLockBuildsParams{UserID: params.UserID})
	if err != nil {
		return nil, fmt.Errorf("service create build: %w", err)
	}

	todayStartTime := time.Now().UTC().Truncate(24 * time.Hour)
	todayEndTime := todayStartTime.Add(24 * time.Hour)

	buildsUsed, err := tx.GetBuildCount(ctx, &DatabaseGetBuildCountParams{
		UserID:    params.UserID,
		StartTime: todayStartTime,
		EndTime:   todayEndTime,
	})
	if err != nil {
		return nil, fmt.Errorf("service create build: %w", err)
	}

	if buildsUsed >= s.config.BuildsAllowed {
		return nil, ErrLimitExceeded
	}

	b, err = tx.CreateBuild(ctx, &DatabaseCreateBuildParams{
		IdempotencyKey: params.IdempotencyKey,
		UserID:         params.UserID,
		DocumentToken:  "document token", // FIXME: remove the stub
	})
	if err != nil {
		return nil, fmt.Errorf("service create build: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("service create build: %w", err)
	}

	return b, nil
}

type GetBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (s *Service) GetBuild(ctx context.Context, params *GetBuildParams) (*build.Build, error) {
	return s.db.GetBuild(ctx, &DatabaseGetBuildParams{
		ID:     params.ID,
		UserID: params.UserID,
	})
}

// TODO: maybe use context.
type WaitForBuildParams struct {
	ID      uuid.UUID
	UserID  uuid.UUID
	Timeout time.Duration
}

func (s *Service) WaitForBuild(ctx context.Context, params *WaitForBuildParams) (*build.Build, error) {
	panic("not implemented")
}

type ListBuildsParams struct {
	Filter    string
	PageSize  int    // zero value (0) means default, constrained, passed to LIMIT
	PageToken string // parsed as int, passed to OFFSET
	UserID    uuid.UUID
}

type ListBuildsResult struct {
	Builds        []*build.Build
	NextPageToken string // zero value ("") means no more pages
	TotalSize     int
}

func (s *Service) ListBuilds(ctx context.Context, params *ListBuildsParams) (*ListBuildsResult, error) {
	panic("not implemented")
}

type CancelBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// CancelBuild.
// It is idempotent without idempotency key.
func (s *Service) CancelBuild(ctx context.Context, params *CancelBuildParams) error {
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

func (s *Service) GetLimits(ctx context.Context, params *GetLimitsParams) (*GetLimitsResult, error) {
	panic("not implemented")
}
