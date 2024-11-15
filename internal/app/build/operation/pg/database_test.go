package pg

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/app/build/operation"
	"github.com/k11v/brick/internal/postgrestest"
	"github.com/k11v/brick/internal/postgresutil"
)

func newDatabase(ctx context.Context, t testing.TB) *Database {
	t.Helper()

	connectionString, teardown, err := postgrestest.Setup(ctx)
	if err != nil {
		t.Fatalf("didn't want %q", err)
	}
	t.Cleanup(func() {
		if teardownErr := teardown(); teardownErr != nil {
			t.Errorf("didn't want %q", teardownErr)
		}
	})

	pool, err := postgresutil.NewPool(ctx, connectionString)
	if err != nil {
		t.Fatalf("didn't want %q", err)
	}

	return NewDatabase(pool)
}

func TestDatabase(t *testing.T) {
	t.Run("creates and gets a build", func(t *testing.T) {
		ctx := context.Background()
		database := newDatabase(ctx, t)
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")

		created, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if err != nil {
			t.Errorf("didn't want %q", err)
		}

		got, err := database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     created.ID,
			UserID: userID,
		})
		if err != nil {
			t.Errorf("didn't want %q", err)
		}

		if want := created; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	// TODO: Consider getting a build instead of relying on CreateBuild's error.
	t.Run("creates a build for unused idempotency key", func(t *testing.T) {
		ctx := context.Background()
		database := newDatabase(ctx, t)
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")

		_, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if err != nil {
			t.Errorf("didn't want %q", err)
		}

		_, err = database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if err != nil {
			t.Errorf("didn't want %q", err)
		}
	})

	t.Run("doesn't create a build for used idempotency key", func(t *testing.T) {
		ctx := context.Background()
		database := newDatabase(ctx, t)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")

		_, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if err != nil {
			t.Errorf("didn't want %q", err)
		}

		_, err = database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if got, want := err, operation.ErrIdempotencyKeyAlreadyUsed; !errors.Is(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}

		_, err = database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  "another document token",
		})
		if got, want := err, operation.ErrIdempotencyKeyAlreadyUsed; !errors.Is(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("doesn't get a build for another user", func(t *testing.T) {
		ctx := context.Background()
		database := newDatabase(ctx, t)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")

		b, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			DocumentToken:  "document token",
		})
		if err != nil {
			t.Errorf("didn't want %q", err)
		}

		_, err = database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     b.ID,
			UserID: uuid.MustParse("dddddddd-0000-0000-0000-000000000000"),
		})
		if got, want := err, operation.ErrNotFound; !errors.Is(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
