package build

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAccessDenied = errors.New("access denied")

type BuildGetter struct {
	DB *pgxpool.Pool // required
}

type BuildGetterGetParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (g *BuildGetter) Get(ctx context.Context, params *BuildGetterGetParams) (*Build, error) {
	b, err := getBuild(ctx, g.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.BuildGetter: %w", err)
	}
	if b.UserID != params.UserID {
		err = ErrAccessDenied
		return nil, fmt.Errorf("build.BuildGetter: %w", err)
	}
	return b, nil
}
