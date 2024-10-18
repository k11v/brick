package build

import (
	"time"

	"github.com/google/uuid"
)

type Service struct {}

type CreateParams struct {
	UserID uuid.UUID
	DocumentFiles map[string][]byte
	CacheKey uuid.UUID
	IdempotencyKey uuid.UUID
}

type CreateResult struct {
	BuildID uuid.UUID
}

func (s *Service) Create(createParams *CreateParams) (*CreateResult, error) {
	// POST /builds
	// check limits for user ID || return error
	// check cache key for user ID || return error
	// insert into builds (new UUID, new date, user ID, idempotency key, status, document files (T), cache key (T)) || return error
	// return build ID
	panic("not implemented")
}

type LimitsParams struct {
	UserID uuid.UUID
}

type LimitsResult struct {
	BuildsUsed int
	BuildsAllowed int
	ResetsAt time.Time
}

func (s *Service) Limits(limitsParams *LimitsParams) (*LimitsResult, error) {
	// GET /builds/limits
	// get limits for user ID
	// return builds used, builds allowed and resets at
	panic("not implemented")
}
