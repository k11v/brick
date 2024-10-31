package build

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
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

	database := NewPostgresDatabase(pool)
	gotDatabaseBuild, err := database.CreateBuild(&DatabaseCreateBuildParams{
		ContextToken:   "",
		DocumentFiles:  make(map[string][]byte),
		IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
		UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
	})
	if err != nil {
		t.Fatalf("didn't want %v", err)
	}
	wantDatabaseBuild := &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}
	if !reflect.DeepEqual(gotDatabaseBuild, wantDatabaseBuild) {
		t.Errorf("didn't want %v", err)
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
