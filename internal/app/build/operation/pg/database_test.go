package pg

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/app/build"
	"github.com/k11v/brick/internal/app/build/operation"
	"github.com/k11v/brick/internal/postgrestest"
	"github.com/k11v/brick/internal/postgresutil"
)

func newDatabase(ctx context.Context, t testing.TB) *Database {
	connectionString, teardown, err := postgrestest.Setup(ctx)
	if err != nil {
		t.Fatalf("didn't want %v", err)
	}
	t.Cleanup(func() {
		if teardownErr := teardown(); teardownErr != nil {
			t.Errorf("didn't want %v", teardownErr)
		}
	})

	pool, err := postgresutil.NewPool(ctx, connectionString)
	if err != nil {
		t.Fatalf("didn't want %v", err)
	}

	return NewDatabase(pool)
}

func TestDatabase(t *testing.T) {
	t.Run("creates and gets a build", func(t *testing.T) {
		ctx := context.Background()
		database := newDatabase(ctx, t)

		databaseBuild, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			ContextToken:   "",
			DocumentFiles:  make(map[string][]byte),
			IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
			UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		})
		if err != nil {
			t.Errorf("didn't want %v", err)
		}

		got, err := database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     databaseBuild.ID,
			UserID: uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		})
		if err != nil {
			t.Errorf("didn't want %v", err)
		}

		if want := databaseBuild; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("doesn't get a build for another user", func(t *testing.T) {
		ctx := context.Background()
		database := newDatabase(ctx, t)

		databaseBuild, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			ContextToken:   "",
			DocumentFiles:  make(map[string][]byte),
			IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
			UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
		})
		if err != nil {
			t.Errorf("didn't want %v", err)
		}

		got, gotErr := database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     databaseBuild.ID,
			UserID: uuid.MustParse("dddddddd-0000-0000-0000-000000000000"),
		})
		if want, wantErr := (*build.Build)(nil), errors.New("access denied"); !reflect.DeepEqual(got, want) || !errors.Is(err, wantErr) {
			t.Logf("got %#v, %#v", got, gotErr)
			t.Errorf("want %#v, %#v", want, wantErr)
		}
	})
}
