package build

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
)

var (
	ErrLimitExceeded             = errors.New("limit exceeded")
	ErrIdempotencyKeyAlreadyUsed = errors.New("idempotency key already used")
)

type Operation struct {
	ID                uuid.UUID
	IdempotencyKey    uuid.UUID
	CreatedAt         time.Time
	UserID            uuid.UUID
	InputDirPrefix    string
	OutputPDFFileKey  string
	ProcessLogFileKey string
	ProcessExitCode   *int
}

type OperationService struct {
	operationsAllowed int

	db *pgxpool.Pool
	mq *amqp091.Connection
	s3 *s3.Client
}

type OperationServiceCreateParams struct {
	UserID         uuid.UUID
	Files          iter.Seq2[File, error]
	IdempotencyKey uuid.UUID
}

func (s *OperationService) Create(ctx context.Context, params *OperationServiceCreateParams) (*Operation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	err = lockBuilds(ctx, tx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}

	todayStartTime := time.Now().UTC().Truncate(24 * time.Hour)
	todayEndTime := todayStartTime.Add(24 * time.Hour)
	operationsUsed, err := getOperationCount(ctx, tx, params.UserID, todayStartTime, todayEndTime)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	if operationsUsed >= s.operationsAllowed {
		err = ErrLimitExceeded
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}

	id := uuid.New()
	operationDirPrefix := fmt.Sprintf("builds/%s", id)
	inputDirPrefix := path.Join(operationDirPrefix, "input")
	outputPDFFileKey := path.Join(operationDirPrefix, "output", "output.pdf")
	processLogFileKey := path.Join(operationDirPrefix, "output", "process.log")
	operation, err := createOperation(ctx, tx, id, params.IdempotencyKey, params.UserID, inputDirPrefix, outputPDFFileKey, processLogFileKey)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	_ = operation

	return &Operation{}, nil
}

type executor interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (commandTag pgconn.CommandTag, err error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

func lockBuilds(ctx context.Context, db executor, userID uuid.UUID) error {
	query := `
		INSERT INTO user_locks (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = excluded.user_id
		RETURNING user_id
	`
	args := []any{userID}

	// TODO: Study pgx.RowTo.
	rows, _ := db.Query(ctx, query, args...)
	_, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}

	return nil
}

func getOperationCount(ctx context.Context, db executor, userID uuid.UUID, startTime, endTime time.Time) (int, error) {
	query := `
		SELECT count(*)
		FROM builds
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`
	args := []any{userID, startTime, endTime}

	rows, _ := db.Query(ctx, query, args...)
	count, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[int])
	if err != nil {
		return 0, err
	}

	return count, nil
}

func createOperation(ctx context.Context, db executor, id uuid.UUID, idempotencyKey uuid.UUID, userID uuid.UUID, inputDirPrefix string, outputPDFFileKey string, processLogFileKey string) (*Operation, error) {
	query := `
		INSERT INTO builds (id, idempotency_key, user_id, input_dir_prefix, output_pdf_file_key, process_log_file_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, idempotency_key, user_id, created_at, input_dir_prefix, output_pdf_file_key, process_log_file_key, process_exit_code
	`
	args := []any{id, idempotencyKey, userID, inputDirPrefix, outputPDFFileKey, processLogFileKey}

	// TODO: Study pgconn.PgError.ColumnName.
	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToOperation)
	if err != nil {
		if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) && pgErr.ColumnName == "idempotency_key" {
			err = ErrIdempotencyKeyAlreadyUsed
		}
		return nil, err
	}

	return b, nil
}

func rowToOperation(collectableRow pgx.CollectableRow) (*Operation, error) {
	type row struct {
		ID                uuid.UUID `db:"id"`
		IdempotencyKey    uuid.UUID `db:"idempotency_key"`
		CreatedAt         time.Time `db:"user_id"`
		UserID            uuid.UUID `db:"created_at"`
		InputDirPrefix    string    `db:"input_dir_prefix"`
		OutputPDFFileKey  string    `db:"output_pdf_file_key"`
		ProcessLogFileKey string    `db:"process_log_file_key"`
		ProcessExitCode   *int      `db:"process_exit_code"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	b := &Operation{
		ID:                collectedRow.ID,
		IdempotencyKey:    collectedRow.IdempotencyKey,
		UserID:            collectedRow.UserID,
		CreatedAt:         collectedRow.CreatedAt,
		InputDirPrefix:    collectedRow.InputDirPrefix,
		OutputPDFFileKey:  collectedRow.OutputPDFFileKey,
		ProcessLogFileKey: collectedRow.ProcessLogFileKey,
		ProcessExitCode:   collectedRow.ProcessExitCode,
	}
	return b, nil
}
