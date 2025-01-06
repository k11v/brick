package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAccessDenied = errors.New("access denied")
	ErrNotDone      = errors.New("not done")
	ErrNotSucceeded = errors.New("not succeeded")
)

type Getter struct {
	DB *pgxpool.Pool // required
	S3 *s3.Client    // required
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

func (g *Getter) GetOutputFile(ctx context.Context, w io.Writer, params *GetterGetParams) error {
	b, err := g.Get(ctx, params)
	if err != nil {
		return err
	}
	if b.Status != StatusSucceeded {
		return fmt.Errorf("build.Getter: %w", ErrNotSucceeded)
	}
	err = downloadFileContent(ctx, g.S3, w, *b.OutputFileKey)
	if err != nil {
		return fmt.Errorf("build.Getter: %w", err)
	}
	return nil
}

func (g *Getter) GetLogFile(ctx context.Context, w io.Writer, params *GetterGetParams) error {
	b, err := g.Get(ctx, params)
	if err != nil {
		return err
	}
	if strings.Split(string(b.Status), ".")[0] != "done" {
		return fmt.Errorf("build.Getter: %w", ErrNotDone)
	}
	err = downloadFileContent(ctx, g.S3, w, *b.LogFileKey)
	if err != nil {
		return fmt.Errorf("build.Getter: %w", err)
	}
	return nil
}
