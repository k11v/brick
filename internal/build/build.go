package build

import (
	"time"

	"github.com/google/uuid"
)

// TODO: add Build.CreatedAt.
type Build struct {
	Done             bool
	Error            error
	ID               uuid.UUID
	NextContextToken string
	OutputFile       []byte
}

type Database interface {
}

type Storage interface {
}

type Broker interface {
}

type Service struct{
	Database Database
	Storage Storage
	Broker Broker
}

type CreateBuildParams struct {
	ContextToken   string
	DocumentFiles  map[string][]byte
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
}

func (s *Service) CreateBuild(createBuildParams *CreateBuildParams) (*Build, error) {
	return &Build{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}, nil
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
