package operation

import (
	"context"

	"github.com/k11v/brick/internal/app/build"
)

const (
	callBegin                    = "Begin"
	callCommit                   = "Commit"
	callCreateBuild              = "CreateBuild"
	callGetBuild                 = "GetBuild"
	callGetBuildByIdempotencyKey = "GetBuildByIdempotencyKey"
	callGetBuildCount            = "GetBuildCount"
	callListBuilds               = "ListBuilds"
	callLockUser                 = "LockUser"
	callRollback                 = "Rollback"
)

type SpyDatabase struct {
	GetBuildCountResult          int
	CreateBuildResult            *build.Build
	GetBuildFunc                 func() (*build.Build, error)
	GetBuildByIdempotencyKeyFunc func() (*build.Build, error)
	ListBuildsResult             *DatabaseListBuildsResult

	Calls *[]string // doesn't contain rolled back calls
}

func (d *SpyDatabase) appendCalls(c ...string) {
	if d.Calls == nil {
		d.Calls = new([]string)
	}
	*d.Calls = append(*d.Calls, c...)
}

func (d *SpyDatabase) CreateBuild(ctx context.Context, params *DatabaseCreateBuildParams) (*build.Build, error) {
	d.appendCalls(callCreateBuild)
	return d.CreateBuildResult, nil
}

func (d *SpyDatabase) GetBuild(ctx context.Context, params *DatabaseGetBuildParams) (*build.Build, error) {
	d.appendCalls(callGetBuild)
	if d.GetBuildFunc == nil {
		return &build.Build{}, nil
	}
	return d.GetBuildFunc()
}

func (d *SpyDatabase) GetBuildByIdempotencyKey(ctx context.Context, params *DatabaseGetBuildByIdempotencyKeyParams) (*build.Build, error) {
	d.appendCalls(callGetBuildByIdempotencyKey)
	if d.GetBuildByIdempotencyKeyFunc == nil {
		return nil, ErrDatabaseNotFound
	}
	return d.GetBuildByIdempotencyKeyFunc()
}

func (d *SpyDatabase) GetBuildCount(ctx context.Context, params *DatabaseGetBuildCountParams) (int, error) {
	d.appendCalls(callGetBuildCount)
	return d.GetBuildCountResult, nil
}

func (d *SpyDatabase) ListBuilds(ctx context.Context, params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	d.appendCalls(callListBuilds)
	return d.ListBuildsResult, nil
}

func (d *SpyDatabase) LockUser(ctx context.Context, params *DatabaseLockUserParams) error {
	d.appendCalls(callLockUser)
	return nil
}

func (d *SpyDatabase) BeginFunc(ctx context.Context, f func(tx Database) error) error {
	d.appendCalls(callBegin)

	tx := *d
	tx.Calls = new([]string)
	if err := f(&tx); err != nil {
		d.appendCalls(callRollback)
		return err
	}

	d.appendCalls(*tx.Calls...)
	d.appendCalls(callCommit)
	return nil
}
