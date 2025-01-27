package build

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAccessDenied  = errors.New("access denied")
	ErrNotDone       = errors.New("not done")
	ErrDoneWithError = errors.New("done with error")
)

type Getter struct {
	DB  *pgxpool.Pool
	STG *s3.Client
}

func NewGetter(db *pgxpool.Pool, stg *s3.Client) *Getter {
	return &Getter{
		DB:  db,
		STG: stg,
	}
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
		return nil, fmt.Errorf("build.Getter: %w", ErrAccessDenied)
	}
	return b, nil
}

func (g *Getter) GetFiles(ctx context.Context, params *GetterGetParams) ([]*File, error) {
	b, err := g.Get(ctx, params)
	if err != nil {
		return nil, err
	}
	files, err := getFiles(ctx, g.DB, b.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Getter: %w", err)
	}
	return files, nil
}

func (g *Getter) CopyOutputData(ctx context.Context, w io.Writer, params *GetterGetParams) error {
	b, err := g.Get(ctx, params)
	if err != nil {
		return err
	}
	if b.Status != StatusDone {
		return fmt.Errorf("build.Getter: %w", ErrNotDone)
	}
	if b.Error != "" {
		return fmt.Errorf("build.Getter: %w", ErrDoneWithError)
	}
	err = downloadData(ctx, g.STG, w, b.OutputDataKey)
	if err != nil {
		return fmt.Errorf("build.Getter: %w", err)
	}
	return nil
}

func (g *Getter) CopyLogData(ctx context.Context, w io.Writer, params *GetterGetParams) error {
	b, err := g.Get(ctx, params)
	if err != nil {
		return err
	}
	if b.Status != StatusDone {
		return fmt.Errorf("build.Getter: %w", ErrNotDone)
	}
	err = downloadData(ctx, g.STG, w, b.LogDataKey)
	if err != nil {
		return fmt.Errorf("build.Getter: %w", err)
	}
	return nil
}
