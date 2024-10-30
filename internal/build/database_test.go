package build

import (
	"context"
	"testing"

	"github.com/k11v/brick/internal/postgrestest"
	"github.com/k11v/brick/internal/postgresutil"
)

func TestPostgresDatabaseBeginFunc(t *testing.T) {
	t.SkipNow()
}

func TestPostgresDatabaseCreateBuild(t *testing.T) {
	ctx := context.Background()

	connectionString, teardown, err := postgrestest.Setup(ctx)
	t.Cleanup(func() {
		if err := teardown(); err != nil {
			t.Errorf("didn't want %v", err)
		}
	})

	pool, err := postgresutil.NewPool(ctx, connectionString)
	if err != nil {
		t.Fatalf("didn't want %v", err)
	}
	if err = pool.Ping(ctx); err != nil {
		t.Errorf("didn't want %v", err)
	}

	_ = pool
}

func TestPostgresDatabaseGetBuild(t *testing.T) {
	t.SkipNow()
}

func TestPostgresDatabaseGetBuildByIdempotencyKey(t *testing.T) {
	t.SkipNow()
}

func TestPostgresDatabaseGetBuildCount(t *testing.T) {
	t.SkipNow()
}

func TestPostgresDatabaseListBuilds(t *testing.T) {
	t.SkipNow()
}

func TestPostgresDatabaseLockUser(t *testing.T) {
	t.SkipNow()
}
