package pg

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/app/build/operation"
	"github.com/k11v/brick/internal/pgutil"
	"github.com/k11v/brick/internal/postgrestest"
)

func NewTestDatabase(tb testing.TB, ctx context.Context) *Database {
	tb.Helper()

	connectionString, teardown, err := postgrestest.Setup(ctx)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}
	tb.Cleanup(func() {
		if teardownErr := teardown(); teardownErr != nil {
			tb.Errorf("didn't want %q", teardownErr)
		}
	})

	pool, err := pgutil.NewPool(ctx, connectionString)
	if err != nil {
		tb.Fatalf("didn't want %q", err)
	}

	return NewDatabase(pool)
}

func TestDatabase(t *testing.T) {
	t.Run("creates and gets a build", func(t *testing.T) {
		ctx := context.Background()
		database := NewTestDatabase(t, ctx)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")
		documentToken := "document token"

		b, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  documentToken,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		got, err := database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     b.ID,
			UserID: userID,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		if want := b; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("doesn't get a build for another user", func(t *testing.T) {
		ctx := context.Background()
		database := NewTestDatabase(t, ctx)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
		documentToken := "document token"

		b, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			DocumentToken:  documentToken,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		_, err = database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     b.ID,
			UserID: uuid.MustParse("dddddddd-0000-0000-0000-000000000000"),
		})
		if got, want := err, operation.ErrNotFound; !errors.Is(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// TODO: Consider getting a build instead of relying on CreateBuild's error.
	t.Run("creates a build for unused idempotency key", func(t *testing.T) {
		ctx := context.Background()
		database := NewTestDatabase(t, ctx)
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")
		documentToken := "document token"

		_, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
			UserID:         userID,
			DocumentToken:  documentToken,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		_, err = database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			UserID:         userID,
			DocumentToken:  documentToken,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
	})

	t.Run("doesn't create a build for used idempotency key", func(t *testing.T) {
		ctx := context.Background()
		database := NewTestDatabase(t, ctx)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")

		_, err := database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		_, err = database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  "document token",
		})
		if got, want := err, operation.ErrIdempotencyKeyAlreadyUsed; !errors.Is(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}

		_, err = database.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  "another document token",
		})
		if got, want := err, operation.ErrIdempotencyKeyAlreadyUsed; !errors.Is(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("gets a build when the build is created in a committed transaction", func(t *testing.T) {
		ctx := context.Background()
		database := NewTestDatabase(t, ctx)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")
		documentToken := "document token"

		tx, err := database.Begin(ctx)
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		t.Cleanup(func() {
			_ = tx.Rollback(ctx)
		})

		createdBuild, err := tx.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  documentToken,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		if err = tx.Commit(ctx); err != nil {
			t.Fatalf("didn't want %q", err)
		}

		gotBuild, err := database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     createdBuild.ID,
			UserID: userID,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		if got, want := gotBuild, createdBuild; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("doesn't get a build when the build is created in a rolled back transaction", func(t *testing.T) {
		ctx := context.Background()
		database := NewTestDatabase(t, ctx)
		idempotencyKey := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
		userID := uuid.MustParse("cccccccc-0000-0000-0000-000000000000")
		documentToken := "document token"

		tx, err := database.Begin(ctx)
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}
		t.Cleanup(func() {
			_ = tx.Rollback(ctx)
		})

		createdBuild, err := tx.CreateBuild(ctx, &operation.DatabaseCreateBuildParams{
			IdempotencyKey: idempotencyKey,
			UserID:         userID,
			DocumentToken:  documentToken,
		})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		if err = tx.Rollback(ctx); err != nil {
			t.Fatalf("didn't want %q", err)
		}

		_, err = database.GetBuild(ctx, &operation.DatabaseGetBuildParams{
			ID:     createdBuild.ID,
			UserID: userID,
		})
		if got, want := err, operation.ErrNotFound; !errors.Is(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
