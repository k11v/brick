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

	"github.com/k11v/brick/internal/appamqp"
	"github.com/k11v/brick/internal/run/runs3"
)

var (
	ErrLimitExceeded             = errors.New("limit exceeded")
	ErrIdempotencyKeyAlreadyUsed = errors.New("idempotency key already used")
	ErrFileTooLarge              = errors.New("file too large")
)

type Build struct {
	ID             uuid.UUID
	CreatedAt      time.Time
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID

	Status        Status
	Error         Error
	ExitCode      int
	LogDataKey    string
	OutputDataKey string
}

type Error string

const (
	ErrorCanceled          Error = "canceled"
	ErrorExitedWithNonZero Error = "exited with non-zero"
)

func ParseError(s string) (errorValue Error, known bool) {
	errorValue = Error(s)
	switch errorValue {
	case ErrorCanceled, ErrorExitedWithNonZero:
		return errorValue, true
	default:
		return errorValue, false
	}
}

type File struct {
	ID      uuid.UUID
	BuildID uuid.UUID

	Name    string
	Type    FileType
	DataKey string
}

type FileType string

const (
	FileTypeRegular   FileType = "regular"
	FileTypeDirectory FileType = "directory"
)

func ParseFileType(s string) (fileType FileType, known bool) {
	fileType = FileType(s)
	switch fileType {
	case FileTypeRegular, FileTypeDirectory:
		return fileType, true
	default:
		return fileType, false
	}
}

type Creator struct {
	DB  *pgxpool.Pool
	MQ  *appamqp.Client
	STG *s3.Client

	BuildsAllowed int
}

type CreatorParams struct {
	BuildsAllowed int
}

func NewCreator(db *pgxpool.Pool, mq *appamqp.Client, stg *s3.Client, params *CreatorParams) *Creator {
	return &Creator{
		DB:            db,
		MQ:            mq,
		STG:           stg,
		BuildsAllowed: params.BuildsAllowed,
	}
}

type CreatorCreateParams struct {
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID

	Files iter.Seq2[*CreatorCreateFileParams, error]
}

type CreatorCreateFileParams struct {
	Name       string
	Type       FileType
	DataReader io.Reader
}

func (c *Creator) Create(ctx context.Context, params *CreatorCreateParams) (*Build, error) {
	tx, err := c.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: Begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Lock builds to get their count.
	err = lockBuilds(ctx, tx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: lockBuilds: %w", err)
	}

	// Check daily quota.
	todayStartTime := time.Now().UTC().Truncate(24 * time.Hour)
	todayEndTime := todayStartTime.Add(24 * time.Hour)
	buildsUsed, err := count(ctx, tx, params.UserID, todayStartTime, todayEndTime)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: count: %w", err)
	}
	if buildsUsed >= c.BuildsAllowed {
		err = ErrLimitExceeded
		return nil, fmt.Errorf("build.Creator: %w", err)
	}

	// Create build.
	b, err := createBuild(ctx, tx, params.IdempotencyKey, params.UserID, "", "")
	if err != nil {
		return nil, fmt.Errorf("build.Creator: createBuild: %w", err)
	}

	// Create object storage keys for output and log files.
	buildDirKey := fmt.Sprintf("builds/%s", b.ID)
	logDataKey := path.Join(buildDirKey, "log")
	outputDataKey := path.Join(buildDirKey, "output.pdf")
	b, err = updateDataKeys(ctx, tx, b.ID, logDataKey, outputDataKey)
	if err != nil {
		return nil, fmt.Errorf("build.Creator: updateDataKeys: %w", err)
	}

	// Create input files and upload their content to object storage.
	inputDirKey := path.Join(buildDirKey, "input")
	filesLen := 0
	for file, err := range params.Files {
		filesLen++
		if err != nil {
			slog.Error("range params.Files", "err", err)
			panic("unimplemented")
		}
		buildInputFile, err := createFile(ctx, tx, b.ID, file.Name, file.Type, "")
		if err != nil {
			slog.Error("createBuildInputFile", "err", err)
			panic("unimplemented")
		}
		if file.Type == FileTypeRegular {
			dataKey := path.Join(inputDirKey, buildInputFile.ID.String())
			buildInputFile, err = updateFileDataKey(ctx, tx, buildInputFile.ID, dataKey)
			if err != nil {
				slog.Error("updateBuildInputFileKey", "err", err)
				panic("unimplemented")
			}
			err = uploadFileData(ctx, c.STG, dataKey, file.DataReader)
			if err != nil {
				slog.Error("uploadFileContent", "err", err)
				panic("unimplemented")
			}
		}
	}
	if filesLen == 0 {
		err = errors.New("files missing")
		return nil, fmt.Errorf("build.Creator: %w", err)
	}

	// Send build created event to workers.
	err = sendCreated(ctx, c.MQ, b)
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

func count(ctx context.Context, db executor, userID uuid.UUID, startTime, endTime time.Time) (int, error) {
	query := `
		SELECT count(*)
		FROM builds
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`
	args := []any{userID, startTime, endTime}

	rows, _ := db.Query(ctx, query, args...)
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[int])
	if err != nil {
		return 0, err
	}

	return c, nil
}

func createBuild(ctx context.Context, db executor, idempotencyKey uuid.UUID, userID uuid.UUID, logDataKey string, outputDataKey string) (*Build, error) {
	query := `
		INSERT INTO builds (idempotency_key, user_id, status, log_data_key, output_data_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, idempotency_key, user_id, status, error, exit_code, log_data_key, output_data_key
	`
	args := []any{idempotencyKey, userID, string(StatusTodo), logDataKey, outputDataKey}

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

func updateDataKeys(ctx context.Context, db executor, id uuid.UUID, logDataKey string, outputDataKey string) (*Build, error) {
	query := `
		UPDATE builds
		SET log_data_key = $2, output_data_key = $3
		WHERE id = $1
		RETURNING id, created_at, idempotency_key, user_id, status, error, exit_code, log_data_key, output_data_key
	`
	args := []any{id, outputDataKey, logDataKey}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func createFile(ctx context.Context, db executor, buildID uuid.UUID, name string, typ FileType, dataKey string) (*File, error) {
	query := `
		INSERT INTO build_files (build_id, name, type, data_key)
		VALUES ($1, $2, $3, $4)
		RETURNING id, build_id, name, type, data_key
	`
	args := []any{buildID, name, string(typ), dataKey}

	rows, _ := db.Query(ctx, query, args...)
	f, err := pgx.CollectExactlyOneRow(rows, rowToFile)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func updateFileDataKey(ctx context.Context, db executor, id uuid.UUID, dataKey string) (*File, error) {
	query := `
		UPDATE build_files
		SET data_key = $2
		WHERE id = $1
		RETURNING id, build_id, name, type, data_key
	`
	args := []any{id, dataKey}

	rows, _ := db.Query(ctx, query, args...)
	f, err := pgx.CollectExactlyOneRow(rows, rowToFile)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// uploadPartSize should be greater than or equal 5MB.
// See github.com/aws/aws-sdk-go-v2/feature/s3/manager.
const uploadPartSize = 10 * 1024 * 1024 // 10MB

func uploadFileData(ctx context.Context, s3Client *s3.Client, key string, r io.Reader) error {
	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = uploadPartSize
	})

	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &runs3.BucketName,
		Key:    &key,
		Body:   r,
	})
	if err != nil {
		if apiErr := smithy.APIError(nil); errors.As(err, &apiErr) && apiErr.ErrorCode() == "EntityTooLarge" {
			err = errors.Join(ErrFileTooLarge, err)
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

func sendCreated(ctx context.Context, mq *appamqp.Client, b *Build) error {
	type message struct {
		ID             uuid.UUID `json:"id"`
		CreatedAt      time.Time `json:"created_at"`
		IdempotencyKey uuid.UUID `json:"idempotency_key"`
		UserID         uuid.UUID `json:"user_id"`

		Status        string `json:"status"`
		Error         string `json:"error"`
		ExitCode      int    `json:"exit_code"`
		LogDataKey    string `json:"log_data_key"`
		OutputDataKey string `json:"output_data_key"`
	}

	msg := message{
		ID:             b.ID,
		CreatedAt:      b.CreatedAt,
		IdempotencyKey: b.IdempotencyKey,
		UserID:         b.UserID,

		Status:        string(b.Status),
		Error:         string(b.Error),
		ExitCode:      b.ExitCode,
		LogDataKey:    b.LogDataKey,
		OutputDataKey: b.OutputDataKey,
	}
	msgBuf := new(bytes.Buffer)
	if err := json.NewEncoder(msgBuf).Encode(msg); err != nil {
		return err
	}

	m := amqp091.Publishing{
		ContentType: "application/json",
		Body:        msgBuf.Bytes(),
	}
	err := mq.Publish(ctx, "", "build.created", false, false, m)
	if err != nil {
		return err
	}

	return nil
}

func rowToBuild(collectableRow pgx.CollectableRow) (*Build, error) {
	type row struct {
		ID             uuid.UUID `db:"id"`
		CreatedAt      time.Time `db:"created_at"`
		IdempotencyKey uuid.UUID `db:"idempotency_key"`
		UserID         uuid.UUID `db:"user_id"`

		Status        string  `db:"status"`
		Error         *string `db:"error"`
		ExitCode      *int    `db:"exit_code"`
		LogDataKey    string  `db:"log_data_key"`
		OutputDataKey string  `db:"output_data_key"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	status, known := ParseStatus(collectedRow.Status)
	if !known {
		slog.Warn("unknown status", "status", status)
	}

	var errorValue Error
	if collectedRow.Error != nil {
		errorValue, known = ParseError(*collectedRow.Error)
		if !known {
			slog.Warn("unknown error", "error", errorValue)
		}
	}

	exitCode := -1
	if collectedRow.ExitCode != nil {
		exitCode = *collectedRow.ExitCode
	}

	return &Build{
		ID:             collectedRow.ID,
		CreatedAt:      collectedRow.CreatedAt,
		IdempotencyKey: collectedRow.IdempotencyKey,
		UserID:         collectedRow.UserID,

		Status:        status,
		Error:         errorValue,
		ExitCode:      exitCode,
		LogDataKey:    collectedRow.LogDataKey,
		OutputDataKey: collectedRow.OutputDataKey,
	}, nil
}

func rowToFile(collectableRow pgx.CollectableRow) (*File, error) {
	type row struct {
		ID      uuid.UUID `db:"id"`
		BuildID uuid.UUID `db:"build_id"`

		Name    string `db:"name"`
		Type    string `db:"type"`
		DataKey string `db:"data_key"`
	}
	collectedRow, err := pgx.RowToStructByName[row](collectableRow)
	if err != nil {
		return nil, err
	}

	typ, known := ParseFileType(collectedRow.Type)
	if !known {
		slog.Warn("unknown file type", "file_type", typ)
	}

	f := &File{
		ID:      collectedRow.ID,
		BuildID: collectedRow.BuildID,

		Name:    collectedRow.Name,
		Type:    typ,
		DataKey: collectedRow.DataKey,
	}
	return f, nil
}
