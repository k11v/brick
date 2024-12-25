package build

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAccessDenied = errors.New("access denied")

type Getter struct {
	DB *pgxpool.Pool // required
}

type GetterGetParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (g *Getter) Get(ctx context.Context, params *GetterGetParams) (*Build, error) {
	b, err := getBuild(ctx, g.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Getter: %w", err)
	}
	if b.UserID != params.UserID {
		err = ErrAccessDenied
		return nil, fmt.Errorf("build.Getter: %w", err)
	}
	return b, nil
}
