package build

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAccessDenied = errors.New("access denied")

type OperationGetter struct {
	DB *pgxpool.Pool // required
}

type OperationGetterGetParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (g *OperationGetter) Get(ctx context.Context, params *OperationGetterGetParams) (*Operation, error) {
	operation, err := getOperation(ctx, g.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.OperationGetter: %w", err)
	}
	if operation.UserID != params.UserID {
		err = ErrAccessDenied
		return nil, fmt.Errorf("build.OperationGetter: %w", err)
	}
	return operation, nil
}
