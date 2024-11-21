package pg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/k11v/brick/internal/buildtask"
)

var _ buildtask.DatabaseTx = (*DatabaseTx)(nil)

type DatabaseTx struct {
	*Database                                    // required
	commitFunc   func(ctx context.Context) error // required
	rollbackFunc func(ctx context.Context) error // required
}

func newDatabaseTx(pgxTx pgx.Tx) *DatabaseTx {
	return &DatabaseTx{
		Database:     NewDatabase(pgxTx),
		commitFunc:   pgxTx.Commit,
		rollbackFunc: pgxTx.Rollback,
	}
}

func (tx *DatabaseTx) Commit(ctx context.Context) error {
	return tx.commitFunc(ctx)
}

func (tx *DatabaseTx) Rollback(ctx context.Context) error {
	return tx.rollbackFunc(ctx)
}
