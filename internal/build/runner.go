package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/run/runs3"
)

var ErrNotFound = errors.New("not found")

type OperationRunner struct {
	DB *pgxpool.Pool // required
	S3 *s3.Client    // required
}

type OperationRunnerRunParams struct {
	ID uuid.UUID
}

func (r *OperationRunner) Run(ctx context.Context, params *OperationRunnerRunParams) (*Operation, error) {
	// Get operation.
	operation, err := getOperation(ctx, r.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("OperationRunner.Run: %w", err)
	}

	// Get operation input files.
	operationInputFiles, err := getOperationInputFiles(ctx, r.DB, operation.ID)
	if err != nil {
		return nil, fmt.Errorf("OperationRunner.Run: %w", err)
	}

	// Create temporary directory.
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("OperationRunner.Run: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Download input files from object storage to disk
	inputDir := filepath.Join(tempDir, "input")
	err = os.MkdirAll(inputDir, 0o777)
	if err != nil {
		return nil, fmt.Errorf("OperationRunner.Run: %w", err)
	}
	for _, operationInputFile := range operationInputFiles {
		downloadFile := func(fileName string, objectKey string) error {
			openFile, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
			if err != nil {
				return err
			}
			defer openFile.Close()

			return downloadFileContent(ctx, r.S3, openFile, objectKey)
		}
		inputFile := filepath.Join(inputDir, operationInputFile.Name)
		err = os.MkdirAll(filepath.Dir(inputFile), 0o777)
		if err != nil {
			return nil, fmt.Errorf("OperationRunner.Run: %w", err)
		}
		err = downloadFile(inputFile, *operationInputFile.ContentKey)
		if err != nil {
			return nil, fmt.Errorf("OperationRunner.Run: %w", err)
		}
	}

	// Run.
	outputDir := filepath.Join(tempDir, "output")
	runResult, err := Run(&RunParams{
		InputDir:  inputDir,
		OutputDir: outputDir,
	})
	if err != nil {
		return nil, fmt.Errorf("build.OperationRunner: %w", err)
	}

	// Upload PDF and log files from disk to object storage.
	uploadFile := func(objectKey string, fileName string) error {
		openFile, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer func() {
			_ = openFile.Close()
		}()

		return uploadFileContent(ctx, r.S3, objectKey, openFile)
	}
	err = uploadFile(*operation.OutputFileKey, runResult.PDFFile)
	if err != nil {
		return nil, fmt.Errorf("build.OperationRunner: %w", err)
	}
	err = uploadFile(*operation.LogFileKey, runResult.LogFile)
	if err != nil {
		return nil, fmt.Errorf("build.OperationRunner: %w", err)
	}

	// Update operation exit code.
	operation, err = updateOperationExitCode(ctx, r.DB, operation.ID, runResult.ExitCode)
	if err != nil {
		return nil, fmt.Errorf("build.OperationRunner: %w", err)
	}

	return operation, nil
}

func getOperation(ctx context.Context, db executor, id uuid.UUID) (*Operation, error) {
	query := `
		SELECT id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code
		FROM operations
		WHERE id = $1
	`
	args := []any{id}

	rows, _ := db.Query(ctx, query, args...)
	o, err := pgx.CollectExactlyOneRow(rows, rowToOperation)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return o, nil
}

func getOperationInputFiles(ctx context.Context, db executor, operationID uuid.UUID) ([]*OperationInputFile, error) {
	query := `
		SELECT id, operation_id, name, content_key
		FROM operation_input_files
		WHERE operation_id = $1
	`
	args := []any{operationID}

	rows, _ := db.Query(ctx, query, args...)
	files, err := pgx.CollectRows(rows, rowToOperationInputFile)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// downloadPartSize should be greater than or equal 5MB.
// See github.com/aws/aws-sdk-go-v2/feature/s3/manager.
const downloadPartSize = 10 * 1024 * 1024 // 10MB

func downloadFileContent(ctx context.Context, s3Client *s3.Client, w io.Writer, key string) error {
	downloader := manager.NewDownloader(s3Client, func(d *manager.Downloader) {
		d.PartSize = int64(downloadPartSize)
		d.Concurrency = 1
	})

	// fakeWriterAt needs manager.Downloader.Concurrency set to 1.
	_, err := downloader.Download(ctx, fakeWriterAt{w}, &s3.GetObjectInput{
		Bucket: &runs3.BucketName,
		Key:    &key,
	})
	if err != nil {
		return err
	}

	return nil
}

// fakeWriterAt wraps an io.Writer to provide a fake WriteAt method.
// This method simply calls w.Write ignoring the offset parameter.
// It can be used with github.com/aws/aws-sdk-go-v2/feature/s3/manager.Downloader.Download
// if its concurrency is set to 1 because this guarantees the sequential writes.
type fakeWriterAt struct {
	w io.Writer // required
}

func (writerAt fakeWriterAt) WriteAt(p []byte, _ int64) (n int, err error) {
	return writerAt.w.Write(p)
}

func updateOperationExitCode(ctx context.Context, db executor, id uuid.UUID, exitCode int) (*Operation, error) {
	query := `
		UPDATE operations
		SET exit_code = $1
		WHERE id = $2
		RETURNING id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code
	`
	args := []any{exitCode, id}

	rows, _ := db.Query(ctx, query, args...)
	o, err := pgx.CollectExactlyOneRow(rows, rowToOperation)
	if err != nil {
		return nil, err
	}

	return o, nil
}
