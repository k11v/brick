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
	KeyFromInputFile  *map[string]string
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
	Files          iter.Seq2[*File, error]
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

	// Lock operations to get their count.
	err = lockOperations(ctx, tx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}

	// Check daily quota.
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

	// Create operation.
	operation, err := createOperation(ctx, tx, params.IdempotencyKey, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	operationDirKey := fmt.Sprintf("builds/%s", operation.ID)
	outputPDFFileKey := path.Join(operationDirKey, "output", "output.pdf")
	processLogFileKey := path.Join(operationDirKey, "output", "process.log")
	operation, err = updateOperation(ctx, tx, operation.ID, outputPDFFileKey, processLogFileKey)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}

	// Create input files and upload their content to object storage.
	inputDirKey := path.Join(operationDirKey, "input")
	for file, err := range params.Files {
		if err != nil {
			// ...
		}
		operationInputFile, err := createOperationInputFile(ctx, tx, operation.ID, file.Name)
		if err != nil {
			// ...
		}
		inputFileKey := path.Join(inputDirKey, operationInputFile.ID.String())
		operationInputFile, err = updateOperationInputFile(ctx, tx, operationInputFile.ID, inputFileKey)
		if err != nil {
			// ...
		}
		err = uploadFile(ctx, s.s3, inputFileKey, file.Content)
		if err != nil {
			// ...
		}
	}

	// Send operation created event to workers.
	err = sendOperationCreated(ctx, s.mq, operation)
	if err != nil {
		// ...
	}

	err = tx.Commit(ctx)
	if err != nil {
		// ...
	}

	return operation, nil
}

type executor interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (commandTag pgconn.CommandTag, err error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

func lockOperations(ctx context.Context, db executor, userID uuid.UUID) error {
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

func createOperation(ctx context.Context, db executor, idempotencyKey uuid.UUID, userID uuid.UUID) (*Operation, error) {
	query := `
		INSERT INTO builds (idempotency_key, user_id)
		VALUES ($1, $2)
		RETURNING id, idempotency_key, user_id, created_at, output_pdf_file_key, process_log_file_key, process_exit_code
	`
	args := []any{idempotencyKey, userID}

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

func updateOperation(ctx context.Context, db executor, id uuid.UUID, outputPDFFileKey string, processLogFileKey string) (*Operation, error) {
	query := `
		UPDATE builds
		SET output_pdf_file_key = $1, process_log_file_key = $2
		WHERE id = $3
		RETURNING id, idempotency_key, user_id, created_at, output_pdf_file_key, process_log_file_key, process_exit_code
	`
	args := []any{outputPDFFileKey, processLogFileKey, id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToOperation)
	if err != nil {
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
		OutputPDFFileKey:  collectedRow.OutputPDFFileKey,
		ProcessLogFileKey: collectedRow.ProcessLogFileKey,
		ProcessExitCode:   collectedRow.ProcessExitCode,
	}
	return b, nil
}
