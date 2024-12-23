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

type BuildRunner struct {
	DB *pgxpool.Pool // required
	S3 *s3.Client    // required
}

type BuildRunnerRunParams struct {
	ID uuid.UUID
}

func (r *BuildRunner) Run(ctx context.Context, params *BuildRunnerRunParams) (*Build, error) {
	// Get build.
	b, err := getBuild(ctx, r.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}

	// Get build input files.
	buildInputFiles, err := getBuildInputFiles(ctx, r.DB, b.ID)
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}

	// Create temporary directory.
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Download input files from object storage to disk
	inputDir := filepath.Join(tempDir, "input")
	err = os.MkdirAll(inputDir, 0o777)
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}
	for _, buildInputFile := range buildInputFiles {
		downloadFile := func(fileName string, objectKey string) error {
			openFile, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
			if err != nil {
				return err
			}
			defer openFile.Close()

			return downloadFileContent(ctx, r.S3, openFile, objectKey)
		}
		inputFile := filepath.Join(inputDir, buildInputFile.Name)
		err = os.MkdirAll(filepath.Dir(inputFile), 0o777)
		if err != nil {
			return nil, fmt.Errorf("build.BuildRunner: %w", err)
		}
		err = downloadFile(inputFile, *buildInputFile.ContentKey)
		if err != nil {
			return nil, fmt.Errorf("build.BuildRunner: %w", err)
		}
	}

	// Run.
	outputDir := filepath.Join(tempDir, "output")
	runResult, err := Run(&RunParams{
		InputDir:  inputDir,
		OutputDir: outputDir,
	})
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
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
	err = uploadFile(*b.OutputFileKey, runResult.PDFFile)
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}
	err = uploadFile(*b.LogFileKey, runResult.LogFile)
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}

	// Update build exit code.
	b, err = updateBuildExitCode(ctx, r.DB, b.ID, runResult.ExitCode)
	if err != nil {
		return nil, fmt.Errorf("build.BuildRunner: %w", err)
	}

	return b, nil
}

func getBuild(ctx context.Context, db executor, id uuid.UUID) (*Build, error) {
	query := `
		SELECT id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code
		FROM builds
		WHERE id = $1
	`
	args := []any{id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return b, nil
}

func getBuildInputFiles(ctx context.Context, db executor, buildID uuid.UUID) ([]*BuildInputFile, error) {
	query := `
		SELECT id, build_id, name, content_key
		FROM build_input_files
		WHERE build_id = $1
	`
	args := []any{buildID}

	rows, _ := db.Query(ctx, query, args...)
	files, err := pgx.CollectRows(rows, rowToBuildInputFile)
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

func updateBuildExitCode(ctx context.Context, db executor, id uuid.UUID, exitCode int) (*Build, error) {
	query := `
		UPDATE builds
		SET exit_code = $1
		WHERE id = $2
		RETURNING id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code
	`
	args := []any{exitCode, id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}
