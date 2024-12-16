package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/buildtask"
	"github.com/k11v/brick/internal/run/runs3"
)

var (
	ErrLimitExceeded             = errors.New("limit exceeded")
	ErrIdempotencyKeyAlreadyUsed = errors.New("idempotency key already used")
)

type Operation struct {
	ID               uuid.UUID
	IdempotencyKey   uuid.UUID
	CreatedAt        time.Time
	UserID           uuid.UUID
	OutputPDFFileID  uuid.UUID
	ProcessLogFileID uuid.UUID
	ProcessExitCode  *int

	InputFiles     []*OperationFile
	OutputPDFFile  *OperationFile
	ProcessLogFile *OperationFile
}

type OperationFile struct {
	ID          uuid.UUID
	OperationID uuid.UUID
	Type        OperationFileType
	Name        string
	ContentKey  string
}

type OperationFileType string

const (
	OperationFileTypeInput  OperationFileType = "input"
	OperationFileTypeOutput OperationFileType = "output"
	OperationFileTypeRun    OperationFileType = "run"
)

// operationFileTypeFromString converts a string to a OperationFileType and checks if it is a known type.
// It returns the OperationFileType and a boolean indicating whether the type is known.
func operationFileTypeFromString(s string) (typ OperationFileType, known bool) {
	typ = OperationFileType(s)
	switch typ {
	case OperationFileTypeInput, OperationFileTypeOutput, OperationFileTypeRun:
		known = true
	default:
		known = false
	}
	return typ, known
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

	// Create output and process files.
	operationDirKey := fmt.Sprintf("operations/%s", operation.ID)
	outputPDFFile, err := createOperationFile(ctx, tx, operation.ID, OperationFileTypeOutput, "output.pdf")
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	outputPDFFileKey := path.Join(operationDirKey, outputPDFFile.ID.String())
	outputPDFFile, err = updateOperationFileContentKey(ctx, tx, outputPDFFile.ID, outputPDFFileKey)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	processLogFile, err := createOperationFile(ctx, tx, operation.ID, OperationFileTypeRun, "process.log")
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	processLogFileKey := path.Join(operationDirKey, processLogFile.ID.String())
	processLogFile, err = updateOperationFileContentKey(ctx, tx, processLogFile.ID, processLogFileKey)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}
	operation, err = updateOperationFileIDs(ctx, tx, operation.ID, outputPDFFile.ID, processLogFile.ID)
	if err != nil {
		return nil, fmt.Errorf("build.OperationService: %w", err)
	}

	// Create input files and upload their content to object storage.
	inputDirKey := path.Join(operationDirKey, "input")
	for file, err := range params.Files {
		if err != nil {
			panic("unimplemented")
		}
		operationFile, err := createOperationFile(ctx, tx, operation.ID, OperationFileTypeInput, file.Name)
		if err != nil {
			panic("unimplemented")
		}
		contentKey := path.Join(inputDirKey, operationFile.ID.String())
		operationFile, err = updateOperationFileContentKey(ctx, tx, operationFile.ID, contentKey)
		if err != nil {
			panic("unimplemented")
		}
		_ = operationFile
		err = uploadFileContent(ctx, s.s3, contentKey, file.Content)
		if err != nil {
			panic("unimplemented")
		}
	}

	// Send operation created event to workers.
	err = sendOperationCreated(ctx, s.mq, operation)
	if err != nil {
		panic("unimplemented")
	}

	err = tx.Commit(ctx)
	if err != nil {
		panic("unimplemented")
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
		FROM operations
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
		INSERT INTO operations (idempotency_key, user_id)
		VALUES ($1, $2)
		RETURNING id, idempotency_key, user_id, created_at, output_pdf_file_id, process_log_file_id, process_exit_code
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

func updateOperationFileIDs(ctx context.Context, db executor, id uuid.UUID, outputPDFFileID uuid.UUID, processLogFileID uuid.UUID) (*Operation, error) {
	query := `
		UPDATE operations
		SET output_pdf_file_id = $1, process_log_file_id = $2
		WHERE id = $3
		RETURNING id, idempotency_key, user_id, created_at, output_pdf_file_id, process_log_file_id, process_exit_code
	`
	args := []any{outputPDFFileID, processLogFileID, id}

	rows, _ := db.Query(ctx, query, args...)
	o, err := pgx.CollectExactlyOneRow(rows, rowToOperation)
	if err != nil {
		return nil, err
	}

	return o, nil
}

func createOperationFile(ctx context.Context, db executor, operationID uuid.UUID, typ OperationFileType, name string) (*OperationFile, error) {
	query := `
		INSERT INTO operation_files (operation_id, type, name)
		VALUES ($1, $2, $3)
		RETURNING id, operation_id, type, name, content_key
	`
	args := []any{operationID, string(typ), name}

	rows, _ := db.Query(ctx, query, args...)
	f, err := pgx.CollectExactlyOneRow(rows, rowToOperationFile)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func updateOperationFileContentKey(ctx context.Context, db executor, id uuid.UUID, contentKey string) (*OperationFile, error) {
	query := `
		UPDATE operation_files
		SET content_key = $1
		WHERE id = $2
		RETURNING id, operation_id, type, name, content_key
	`
	args := []any{contentKey, id}

	rows, _ := db.Query(ctx, query, args...)
	f, err := pgx.CollectExactlyOneRow(rows, rowToOperationFile)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// uploadPartSize should be greater than or equal 5MB.
// See github.com/aws/aws-sdk-go-v2/feature/s3/manager.
const uploadPartSize = 10 * 1024 * 1024 // 10MB

func uploadFileContent(ctx context.Context, s3Client *s3.Client, key string, content io.Reader) error {
	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = uploadPartSize
	})

	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &runs3.BucketName,
		Key:    &key,
		Body:   content,
	})
	if err != nil {
		if apiErr := smithy.APIError(nil); errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" {
			err = errors.Join(buildtask.FileTooLarge, err)
		}
		return err
	}

	err = s3.NewObjectExistsWaiter(s3Client).Wait(ctx, &s3.HeadObjectInput{
		Bucket: &runs3.BucketName,
		Key:    &key,
	}, time.Minute)
	if err != nil {
		return err
	}

	return nil
}

func sendOperationCreated(ctx context.Context, mq *amqp091.Connection, operation *Operation) error {
	type message struct {
		ID               uuid.UUID `json:"id"`
		IdempotencyKey   uuid.UUID `json:"idempotency_key"`
		CreatedAt        time.Time `json:"created_at"`
		UserID           uuid.UUID `json:"user_id"`
		OutputPDFFileID  uuid.UUID `json:"output_pdf_file_id"`
		ProcessLogFileID uuid.UUID `json:"process_log_file_id"`
		ProcessExitCode  *int      `json:"process_exit_code"`
	}
	msg := message{
		ID:               operation.ID,
		IdempotencyKey:   operation.IdempotencyKey,
		CreatedAt:        operation.CreatedAt,
		UserID:           operation.UserID,
		OutputPDFFileID:  operation.OutputPDFFileID,
		ProcessLogFileID: operation.ProcessLogFileID,
		ProcessExitCode:  operation.ProcessExitCode,
	}
	msgBuf := new(bytes.Buffer)
	if err := json.NewEncoder(msgBuf).Encode(msg); err != nil {
		return err
	}

	ch, err := mq.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	q, err := ch.QueueDeclare("operation.created", false, false, false, false, nil)
	if err != nil {
		return err
	}

	m := amqp091.Publishing{
		ContentType: "application/json",
		Body:        msgBuf.Bytes(),
	}
	err = ch.PublishWithContext(ctx, "", q.Name, false, false, m)
	if err != nil {
		return err
	}

	return nil
}

func rowToOperation(collectableRow pgx.CollectableRow) (*Operation, error) {
	type row struct {
		ID               uuid.UUID `db:"id"`
		IdempotencyKey   uuid.UUID `db:"idempotency_key"`
		CreatedAt        time.Time `db:"created_at"`
		UserID           uuid.UUID `db:"user_id"`
		OutputPDFFileID  uuid.UUID `db:"output_pdf_file_id"`
		ProcessLogFileID uuid.UUID `db:"process_log_file_id"`
		ProcessExitCode  *int      `db:"process_exit_code"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	o := &Operation{
		ID:               collectedRow.ID,
		IdempotencyKey:   collectedRow.IdempotencyKey,
		CreatedAt:        collectedRow.CreatedAt,
		UserID:           collectedRow.UserID,
		OutputPDFFileID:  collectedRow.OutputPDFFileID,
		ProcessLogFileID: collectedRow.ProcessLogFileID,
		ProcessExitCode:  collectedRow.ProcessExitCode,
	}
	return o, nil
}

func rowToOperationFile(collectableRow pgx.CollectableRow) (*OperationFile, error) {
	type row struct {
		ID          uuid.UUID `db:"id"`
		OperationID uuid.UUID `db:"operation_id"`
		Type        string    `db:"type"`
		Name        string    `db:"name"`
		ContentKey  string    `db:"content_key"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	typ, known := operationFileTypeFromString(collectedRow.Type)
	if !known {
		slog.Warn(
			"got unknown OperationFileType from the database",
			"operation_file_id", collectedRow.ID,
			"type", collectedRow.Type,
		)
	}

	f := &OperationFile{
		ID:          collectedRow.ID,
		OperationID: collectedRow.OperationID,
		Type:        typ,
		Name:        collectedRow.Name,
		ContentKey:  collectedRow.ContentKey,
	}
	return f, nil
}
