package build

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrLimitExceeded = errors.New("limit exceeded")

// TODO: add Build.CreatedAt.
type Build struct {
	Done             bool
	Error            error
	ID               uuid.UUID
	NextContextToken string
	OutputFile       []byte
}

type Database interface {
	Begin() (tx Database, commit func() error, rollback func() error, err error)
	LockUser(params *DatabaseLockUserParams) error
	GetBuildCount(params *DatabaseGetBuildCountParams) (int, error)
	CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error)
	GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error)
	ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error)
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

func (s *Service) CreateBuild(createBuildParams *CreateBuildParams) (*Build, error) {
	tx, commit, rollback, err := s.database.Begin()
	if err != nil {
		return nil, err
	}
	defer rollback()

	if err = tx.LockUser(&DatabaseLockUserParams{UserID: createBuildParams.UserID}); err != nil {
		return nil, err
	}

	startTime := time.Now().UTC().Truncate(24 * time.Hour)
	endTime := startTime.Add(24 * time.Hour)

	used, err := tx.GetBuildCount(&DatabaseGetBuildCountParams{
		UserID:    createBuildParams.UserID,
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		return nil, err
	}

	if used >= s.config.BuildsAllowed {
		return nil, ErrLimitExceeded
	}

	databaseBuild, err := tx.CreateBuild(&DatabaseCreateBuildParams{
		ContextToken:   createBuildParams.ContextToken,
		DocumentFiles:  createBuildParams.DocumentFiles,
		IdempotencyKey: createBuildParams.IdempotencyKey,
		UserID:         createBuildParams.UserID,
	})
	if err != nil {
		return nil, err
	}

	commit()
	return buildFromDatabaseBuild(databaseBuild), nil
}

type GetBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (s *Service) GetBuild(getBuildParams *GetBuildParams) (*Build, error) {
	panic("not implemented")
}

// TODO: maybe use context.
type GetBuildWithTimeout struct {
	ID      uuid.UUID
	Timeout time.Duration
	UserID  uuid.UUID
}

func (s *Service) GetBuildWithTimeout(getBuildWithTimeoutParams *GetBuildWithTimeout) (*Build, error) {
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

func (s *Service) ListBuilds(listBuildsParams *ListBuildsParams) (*ListBuildsResult, error) {
	panic("not implemented")
}

type CancelBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// CancelBuild.
// It is idempotent without idempotency key.
func (s *Service) CancelBuild(cancelBuildParams *CancelBuildParams) error {
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

func (s *Service) GetLimits(getLimitsParams *GetLimitsParams) (*GetLimitsResult, error) {
	panic("not implemented")
}

func buildFromDatabaseBuild(databaseBuild *DatabaseBuild) *Build {
	return &Build{
		Done:             databaseBuild.Done,
		Error:            databaseBuild.Error,
		ID:               databaseBuild.ID,
		NextContextToken: databaseBuild.NextContextToken,
		OutputFile:       databaseBuild.OutputFile,
	}
}
