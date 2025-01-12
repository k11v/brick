package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/k11v/brick/internal/run/runs3"
)

var ErrNotFound = errors.New("not found")

type Runner struct {
	DB *pgxpool.Pool // required
	S3 *s3.Client    // required
}

type RunnerRunParams struct {
	ID uuid.UUID
}

func (r *Runner) Run(ctx context.Context, params *RunnerRunParams) (*Build, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Get build for status update to running.
	b, err := getBuildForUpdate(ctx, r.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	// If build is running, return.
	if strings.Split(string(b.Status), ".")[0] == "running" {
		return nil, fmt.Errorf("build.Runner: %w", ErrAlreadyRunning)
	}

	// If build is done, return.
	if strings.Split(string(b.Status), ".")[0] == "done" {
		return nil, fmt.Errorf("build.Runner: %w", ErrAlreadyDone)
	}

	// Update build status to running.
	b, err = updateBuildStatus(ctx, tx, params.ID, StatusRunning)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	// Get build input files.
	buildInputFiles, err := getBuildInputFiles(ctx, r.DB, b.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	// Prepare reader with input tar.
	// TODO: Implement real inputTarReader.
	var inputTarReader io.Reader
	{
		// Create temporary directory.
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			return nil, fmt.Errorf("build.Runner: %w", err)
		}
		defer func() {
			_ = os.RemoveAll(tempDir)
		}()

		// Download input files from object storage to disk
		inputDir := filepath.Join(tempDir, "input")
		err = os.MkdirAll(inputDir, 0o777)
		if err != nil {
			return nil, fmt.Errorf("build.Runner: %w", err)
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
				return nil, fmt.Errorf("build.Runner: %w", err)
			}
			err = downloadFile(inputFile, *buildInputFile.ContentKey)
			if err != nil {
				return nil, fmt.Errorf("build.Runner: %w", err)
			}
		}

		inputTarReader, err = os.Open(".run/main.tar")
		if err != nil {
			return nil, fmt.Errorf("build.Runner: %w", err)
		}
	}

	// Run.
	var result struct{ ExitCode int }
	err = func() error {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return err
		}

		// Create volume.
		vol, err := cli.VolumeCreate(ctx, volume.CreateOptions{})
		if err != nil {
			return err
		}
		defer func() {
			err = cli.VolumeRemove(ctx, vol.Name, false)
			if err != nil {
				slog.Error("didn't remove volume", "id", vol.Name, "error", err)
			}
		}()

		// Create untar input container.
		untarInputCont, err := cli.ContainerCreate(
			ctx,
			&container.Config{
				Image:        "brick-build",
				Entrypoint:   strslice.StrSlice{},
				Cmd:          strslice.StrSlice{"sh", "-c", `mkdir /user/run/input && cd /user/run/input && exec tar -v -x`},
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
				OpenStdin:    true,
				StdinOnce:    true,
			},
			&container.HostConfig{
				NetworkMode:    "none",
				CapDrop:        strslice.StrSlice{"ALL"},
				CapAdd:         strslice.StrSlice{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FSETID", "CAP_FOWNER", "CAP_MKNOD", "CAP_NET_RAW", "CAP_SETGID", "CAP_SETUID", "CAP_SETFCAP", "CAP_SETPCAP", "CAP_NET_BIND_SERVICE", "CAP_SYS_CHROOT", "CAP_KILL", "CAP_AUDIT_WRITE"},
				ReadonlyRootfs: true,
				Mounts: []mount.Mount{{
					Type:   mount.TypeVolume,
					Source: vol.Name,
					Target: "/user/run",
				}},
				LogConfig: container.LogConfig{
					Type: "none",
				},
			},
			nil,
			nil,
			"",
		)
		if err != nil {
			return err
		}
		defer func() {
			err = cli.ContainerRemove(ctx, untarInputCont.ID, container.RemoveOptions{})
			if err != nil {
				slog.Error("didn't remove container", "id", untarInputCont.ID, "error", err)
			}
		}()

		// TODO: Consider DetachKeys.
		untarInputContConn, err := cli.ContainerAttach(ctx, untarInputCont.ID, container.AttachOptions{
			Stream:     true,
			Stdin:      true,
			Stdout:     true,
			Stderr:     true,
			DetachKeys: "",
		})
		if err != nil {
			return err
		}
		defer untarInputContConn.Close()

		err = cli.ContainerStart(ctx, untarInputCont.ID, container.StartOptions{})
		if err != nil {
			return err
		}

		stdinErrCh := make(chan error, 1)
		go func() {
			defer close(stdinErrCh)
			_, err := io.Copy(untarInputContConn.Conn, inputTarReader)
			if err != nil {
				stdinErrCh <- err
				return
			}
			err = untarInputContConn.CloseWrite()
			if err != nil {
				stdinErrCh <- err
				return
			}
		}()

		var stdoutBuffer, stderrBuffer bytes.Buffer
		_, err = stdcopy.StdCopy(&stdoutBuffer, &stderrBuffer, untarInputContConn.Conn)
		if err != nil {
			return err
		}

		err = <-stdinErrCh
		if err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	// Update build exit code.
	b, err = updateBuildExitCode(ctx, r.DB, b.ID, result.ExitCode)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	// Update build status to done.
	var doneStatus Status
	if result.ExitCode == 0 {
		doneStatus = StatusSucceeded
	} else {
		doneStatus = StatusFailed
	}
	b, err = updateBuildStatus(ctx, r.DB, b.ID, doneStatus)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	return b, nil
}

func getBuild(ctx context.Context, db executor, id uuid.UUID) (*Build, error) {
	query := `
		SELECT id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code, status
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

func getBuildInputFiles(ctx context.Context, db executor, buildID uuid.UUID) ([]*InputFile, error) {
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
		RETURNING id, idempotency_key, user_id, created_at, output_file_key, log_file_key, exit_code, status
	`
	args := []any{exitCode, id}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}
