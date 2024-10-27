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
	TransactWithUser(userID uuid.UUID) (database Database, commit func() error, rollback func() error, err error)
	GetBuildCount(userID uuid.UUID, startTime time.Time, endTime time.Time) (int, error)
	CreateBuild(contextToken string, documentFiles map[string][]byte, idempotencyKey uuid.UUID, userID uuid.UUID) (*Build, error)
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
	database, commit, rollback, err := s.database.TransactWithUser(createBuildParams.UserID)
	if err != nil {
		return nil, err
	}
	defer rollback()

	startTime := time.Now().Truncate(24 * time.Hour)
	endTime := startTime.Add(24 * time.Hour)

	used, err := database.GetBuildCount(createBuildParams.UserID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	if used >= s.config.BuildsAllowed {
		return nil, ErrLimitExceeded
	}

	b, err := database.CreateBuild(
		createBuildParams.ContextToken,
		createBuildParams.DocumentFiles,
		createBuildParams.IdempotencyKey,
		createBuildParams.UserID,
	)
	if err != nil {
		return nil, err
	}

	commit()
	return b, nil
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
