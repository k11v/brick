package buildtask

import (
	"context"

	"github.com/k11v/brick/internal/build"
)

const (
	callBegin                    = "Begin"
	callCommit                   = "Commit"
	callCreateBuild              = "CreateBuild"
	callGetBuild                 = "GetBuild"
	callGetBuildByIdempotencyKey = "GetBuildByIdempotencyKey"
	callGetBuildCount            = "GetBuildCount"
	callListBuilds               = "ListBuilds"
	callLockBuilds               = "LockBuilds"
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

type SpyDatabaseTx struct {
	*SpyDatabase
	CommitFunc   func() error
	RollbackFunc func() error
}

func (tx *SpyDatabaseTx) Commit(ctx context.Context) error {
	return tx.CommitFunc()
}

func (tx *SpyDatabaseTx) Rollback(ctx context.Context) error {
	return tx.RollbackFunc()
}

func (d *SpyDatabase) Begin(ctx context.Context) (DatabaseTx, error) {
	d.appendCalls(callBegin)

	txDatabase := *d
	txDatabase.Calls = new([]string)

	tx := &SpyDatabaseTx{
		SpyDatabase: &txDatabase,
		CommitFunc: func() error {
			d.appendCalls(*txDatabase.Calls...)
			d.appendCalls(callCommit)
			return nil
		},
		RollbackFunc: func() error {
			d.appendCalls(callRollback)
			return nil
		},
	}
	return tx, nil
}

func (d *SpyDatabase) LockBuilds(ctx context.Context, params *DatabaseLockBuildsParams) error {
	d.appendCalls(callLockBuilds)
	return nil
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
