package build

import (
	"context"
	"reflect"
	"slices"
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
	if err != nil {
		t.Fatalf("didn't want %v", err)
	}
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
		t.Fatalf("didn't want %v", err)
	}

	rows, err := pool.Query(context.Background(), `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public'`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	tableNames := make([]string, 0)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("row scan failed: %v", err)
		}
		tableNames = append(tableNames, tableName)
	}
	if rows.Err() != nil {
		t.Fatalf("row iteration error: %v", rows.Err())
	}

	got, want := tableNames, []string{"builds", "schema_migrations"}
	if !reflect.DeepEqual(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Errorf("got %v, want %v", got, want)
	}
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
