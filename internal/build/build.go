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

type Service struct{}

type CreateBuildParams struct {
	ContextFiles   map[string][]byte
	ContextToken   string
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
}

func (s *Service) CreateBuild(createBuildParams *CreateBuildParams) (*Build, error) {
	// POST /builds
	//
	// IdempotencyKey will be read from X-Idempotency-Key header field.
	// Draft for idempotency key exists but it is not accepted yet.
	// See https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/.
	//
	// check limits for user ID || return error
	// check cache key for user ID || return error
	// select cached files from caches using cache key
	// insert into builds (new UUID, new date, user ID, idempotency key, status, input files (T), cached files (T)) || return error
	// return build ID
	//
	// Regarding late force: DELETE /caches/{cache-key}
	// check cache key for user ID || return error
	// delete from caches using cache key || return error (endpoint should return 404 if not found)
	// return nil (endpoint should return 201 if found and deleted)
	//
	// Regarding late cache: cache can be invalidated manually using cache key.
	// Cache can be invalidated automatically when too much time passed.
	// Cache can be invalidated automatically when too much space used.
	// When cache is invalidated, it is deleted.
	panic("not implemented")
}

type GetBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (s *Service) GetBuild(getBuildParams *GetBuildParams) (*Build, error) {
	// GET /builds/{id}
	panic("not implemented")
}

// TODO: maybe use context.
type GetBuildWithTimeout struct {
	ID      uuid.UUID
	Timeout time.Duration
	UserID  uuid.UUID
}

func (s *Service) GetBuildWithTimeout(getBuildWithTimeoutParams *GetBuildWithTimeout) (*Build, error) {
	// POST /builds/{id}/wait
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
	// GET /builds
	panic("not implemented")
}

type CancelBuildParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (s *Service) CancelBuild(cancelBuildParams *CancelBuildParams) error {
	// POST /builds/{id}/cancel
	// Idempotency key is not used because the cancel operation is idempotent.
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
	// GET /builds/limits
	// get limits for user ID
	// return builds used, builds allowed and resets at
	panic("not implemented")
}
