package build

import (
	"time"

	"github.com/google/uuid"
)

// cache can be invalidated manually using cache key.
// cache can be invalidated automatically when too much time passed.
// cache can be invalidated automatically when too much space used.
// when cache is invalidated, it is deleted.

type Service struct{}

type Build struct {
	Done       bool
	OutputFile []byte
	Err        error
}

type CreateParams struct {
	UserID         uuid.UUID
	InputFiles     map[string][]byte
	CacheKey       uuid.UUID
	IdempotencyKey uuid.UUID
}

type CreateResult struct {
	BuildID uuid.UUID
}

func (s *Service) Create(createParams *CreateParams) (*CreateResult, error) {
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
	panic("not implemented")
}

type GetParams struct {
	ID uuid.UUID
}

func (s *Service) Get(getParams *GetParams) (*Build, error) {
	// GET /builds/{id}
	panic("not implemented")
}

type ListParams struct {
	Filter    string
	PageToken string // parsed as int, passed to OFFSET
	PageSize  int    // zero value (0) means default, constrained, passed to LIMIT
}

type ListResult struct {
	Builds        []*Build
	NextPageToken string // zero value ("") means no more pages
	TotalSize     int
}

func (s *Service) List(listParams *ListParams) (*ListResult, error) {
	// GET /builds
	panic("not implemented")
}

type CancelParams struct {
	ID uuid.UUID
}

type CancelResult struct{}

func (s *Service) Cancel(cancelParams *CancelParams) error {
	// POST /builds/{id}/cancel
	// Idempotency key is not used because the cancel operation is idempotent.
	panic("not implemented")
}

type WaitParams struct {
	Timeout time.Duration
}

func (s *Service) Wait(waitParams *WaitParams) (*Build, error) {
	// POST /builds/{id}/wait
	panic("not implemented")
}

type LimitsParams struct {
	UserID uuid.UUID
}

type LimitsResult struct {
	BuildsUsed    int
	BuildsAllowed int
	ResetsAt      time.Time
}

func (s *Service) Limits(limitsParams *LimitsParams) (*LimitsResult, error) {
	// GET /builds/limits
	// get limits for user ID
	// return builds used, builds allowed and resets at
	panic("not implemented")
}

type DeleteCacheParams struct {
	UserID   uuid.UUID
	CacheKey uuid.UUID
}

func (s *Service) DeleteCache(deleteCacheParams *DeleteCacheParams) error {
	// DELETE /caches/{cache-key}
	// check cache key for user ID || return error
	// delete from caches using cache key || return error (endpoint should return 404 if not found)
	// return nil (endpoint should return 201 if found and deleted)
	panic("not implemented")
}
