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

type Build struct {
	ID             uuid.UUID
	IdempotencyKey uuid.UUID
	CreatedAt      time.Time
	UserID         uuid.UUID
	OutputFileKey  *string
	LogFileKey     *string
	ExitCode       *int
	Status         Status
}

type InputFile struct {
	ID         uuid.UUID
	BuildID    uuid.UUID
	Name       string
	ContentKey *string
}

type Creator struct {
	DB *pgxpool.Pool       // required
	MQ *amqp091.Connection // required
	S3 *s3.Client          // required

	BuildsAllowed int
}

type CreatorCreateParams struct {
	UserID         uuid.UUID
	Files          iter.Seq2[*File, error]
	IdempotencyKey uuid.UUID
}

type File struct {
	Name string
	Type string
	Data io.Reader
}

func (c *Creator) Create(ctx context.Context, params *CreatorCreateParams) (*Build, error) {
	tx, err := c.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Lock builds to get their count.
	err = lockBuilds(ctx, tx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: %w", err)
	}

	// Check daily quota.
	todayStartTime := time.Now().UTC().Truncate(24 * time.Hour)
	todayEndTime := todayStartTime.Add(24 * time.Hour)
	buildsUsed, err := getBuildCount(ctx, tx, params.UserID, todayStartTime, todayEndTime)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: %w", err)
	}
	if buildsUsed >= c.BuildsAllowed {
		err = ErrLimitExceeded
		return nil, fmt.Errorf("build.Creator: %w", err)
	}

	// Create build.
	b, err := createBuild(ctx, tx, params.IdempotencyKey, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: %w", err)
	}

	// Create object storage keys for output and log files.
	buildDirKey := fmt.Sprintf("builds/%s", b.ID)
	outputFileKey := path.Join(buildDirKey, "output.pdf")
	logFileKey := path.Join(buildDirKey, "log")
	b, err = updateBuildContentKeys(ctx, tx, b.ID, outputFileKey, logFileKey)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: %w", err)
	}

	// Create input files and upload their content to object storage.
	inputDirKey := path.Join(buildDirKey, "input")
	for file, err := range params.Files {
		if err != nil {
			slog.Error("range params.Files", "err", err)
			panic("unimplemented")
		}
		buildInputFile, err := createBuildInputFile(ctx, tx, b.ID, file.Name)
		if err != nil {
			slog.Error("createBuildInputFile", "err", err)
			panic("unimplemented")
		}
		contentKey := path.Join(inputDirKey, buildInputFile.ID.String())
		buildInputFile, err = updateBuildInputFileKey(ctx, tx, buildInputFile.ID, contentKey)
		if err != nil {
			slog.Error("updateBuildInputFileKey", "err", err)
			panic("unimplemented")
		}
		_ = buildInputFile
		err = uploadFileContent(ctx, c.S3, contentKey, file.Data)
		if err != nil {
			slog.Error("uploadFileContent", "err", err)
			panic("unimplemented")
		}
	}

	// Send build created event to workers.
	err = sendBuildCreated(ctx, c.MQ, b)
	if err != nil {
		slog.Error("sendBuildCreated", "err", err)
		panic("unimplemented")
	}

	err = tx.Commit(ctx)
	if err != nil {
		slog.Error("Commit", "err", err)
		panic("unimplemented")
	}

	return b, nil
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

func getBuildCount(ctx context.Context, db executor, userID uuid.UUID, startTime, endTime time.Time) (int, error) {
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

func createBuild(ctx context.Context, db executor, idempotencyKey uuid.UUID, userID uuid.UUID) (*Build, error) {
	query := `
		INSERT INTO builds (idempotency_key, user_id, status)
		VALUES ($1, $2, $3)
		RETURNING id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code, status
	`
	args := []any{idempotencyKey, userID, string(StatusPending)}

	// TODO: Study pgconn.PgError.ColumnName.
	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) && pgErr.ColumnName == "idempotency_key" {
			err = ErrIdempotencyKeyAlreadyUsed
		}
		return nil, err
	}

	return b, nil
}

func updateBuildContentKeys(ctx context.Context, db executor, id uuid.UUID, outputFileKey string, logFileKey string) (*Build, error) {
	query := `
		UPDATE builds
		SET output_file_key = $1, log_file_key = $2
		WHERE id = $3
		RETURNING id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code, status
	`
	args := []any{outputFileKey, logFileKey, id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func createBuildInputFile(ctx context.Context, db executor, buildID uuid.UUID, name string) (*InputFile, error) {
	query := `
		INSERT INTO build_input_files (build_id, name)
		VALUES ($1, $2)
		RETURNING id, build_id, name, content_key
	`
	args := []any{buildID, name}

	rows, _ := db.Query(ctx, query, args...)
	f, err := pgx.CollectExactlyOneRow(rows, rowToBuildInputFile)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func updateBuildInputFileKey(ctx context.Context, db executor, id uuid.UUID, contentKey string) (*InputFile, error) {
	query := `
		UPDATE build_input_files
		SET content_key = $1
		WHERE id = $2
		RETURNING id, build_id, name, content_key
	`
	args := []any{contentKey, id}

	rows, _ := db.Query(ctx, query, args...)
	f, err := pgx.CollectExactlyOneRow(rows, rowToBuildInputFile)
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

func sendBuildCreated(ctx context.Context, mq *amqp091.Connection, b *Build) error {
	type message struct {
		ID             uuid.UUID `json:"id"`
		IdempotencyKey uuid.UUID `json:"idempotency_key"`
		CreatedAt      time.Time `json:"created_at"`
		UserID         uuid.UUID `json:"user_id"`
		OutputFileKey  *string   `json:"output_file_key"`
		LogFileKey     *string   `json:"log_file_key"`
		ExitCode       *int      `json:"exit_code"`
	}
	msg := message{
		ID:             b.ID,
		IdempotencyKey: b.IdempotencyKey,
		CreatedAt:      b.CreatedAt,
		UserID:         b.UserID,
		OutputFileKey:  b.OutputFileKey,
		LogFileKey:     b.LogFileKey,
		ExitCode:       b.ExitCode,
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

	q, err := ch.QueueDeclare("build.created", false, false, false, false, nil)
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

func rowToBuild(collectableRow pgx.CollectableRow) (*Build, error) {
	type row struct {
		ID             uuid.UUID `db:"id"`
		IdempotencyKey uuid.UUID `db:"idempotency_key"`
		CreatedAt      time.Time `db:"created_at"`
		UserID         uuid.UUID `db:"user_id"`
		OutputFileKey  *string   `db:"output_file_key"`
		LogFileKey     *string   `db:"log_file_key"`
		ExitCode       *int      `db:"exit_code"`
		Status         string    `db:"status"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	b := &Build{
		ID:             collectedRow.ID,
		IdempotencyKey: collectedRow.IdempotencyKey,
		CreatedAt:      collectedRow.CreatedAt,
		UserID:         collectedRow.UserID,
		OutputFileKey:  collectedRow.OutputFileKey,
		LogFileKey:     collectedRow.LogFileKey,
		ExitCode:       collectedRow.ExitCode,
		Status:         Status(collectedRow.Status), // TODO: Warn if unknown.
	}
	return b, nil
}

func rowToBuildInputFile(collectableRow pgx.CollectableRow) (*InputFile, error) {
	type row struct {
		ID         uuid.UUID `db:"id"`
		BuildID    uuid.UUID `db:"build_id"`
		Name       string    `db:"name"`
		ContentKey *string   `db:"content_key"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	f := &InputFile{
		ID:         collectedRow.ID,
		BuildID:    collectedRow.BuildID,
		Name:       collectedRow.Name,
		ContentKey: collectedRow.ContentKey,
	}
	return f, nil
}
