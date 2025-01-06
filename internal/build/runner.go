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

	// FIXME:
	//
	// We probably want only one runner to be entering the next stage.
	// Otherwise multiple runners may try to write output and log files
	// and the result might be undetermined.
	// So additionally we could check if build status is not "running".
	// But if it is running it is not necessarily true, maybe we have set
	// the status and crashed.
	//
	// Also if it fails (by simply returning an error), we are stuck in running.

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

	// Run.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	createResp, err := cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:        "brick-runner",
			Cmd:          strslice.StrSlice{"-i", "main.md", "-o", "/user/run/output/main.pdf", "-c", "/user/run/output/cache"},
			WorkingDir:   "/user/run/input",
			AttachStderr: true,
			AttachStdout: true,
		},
		&container.HostConfig{
			NetworkMode: "none",
			CapDrop:     strslice.StrSlice{"ALL"},
			CapAdd: strslice.StrSlice{ // https://github.com/moby/moby/blob/master/oci/caps/defaults.go#L6-L19
				"CAP_CHOWN",
				"CAP_DAC_OVERRIDE",
				"CAP_FSETID",
				"CAP_FOWNER",
				"CAP_MKNOD",
				"CAP_NET_RAW",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_SETFCAP",
				"CAP_SETPCAP",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_CHROOT",
				"CAP_KILL",
				"CAP_AUDIT_WRITE",
			},
			ReadonlyRootfs: true,
			Mounts: []mount.Mount{{
				Type:   mount.TypeTmpfs,
				Target: "/user/run",
				TmpfsOptions: &mount.TmpfsOptions{
					SizeBytes: 256 * 1024 * 1024, // 256MB
					Mode:      0o1777,            // TODO: Check
				},
			}},

			// CgroupnsMode: container.CgroupnsModePrivate,                 // Likely keep. It is likely the default.
			// IpcMode:      container.IPCModePrivate,                      // Likely keep. It is likely the default. Likely stricter IPCModeNone could be considered.
			// OomScoreAdj:  500,                                           // Maybe keep. Maybe change value. It likely controls the likelyhood of this container getting killed in OOM scenario.
			// PidMode:      "private",                                     // Likely keep. Maybe change. It is maybe the default.
			// Privileged:   false,                                         // Likely keep. It is the default.
			// SecurityOpt:  nil,                                           // Maybe keep but change value. It is related to SELinux.
			// StorageOpt:   nil,                                           // Maybe keep. It is related to storage.
			// Tmpfs:        map[string]string{"/user/run": "size=256m"}, // Maybe keep. Maybe change.
			// UTSMode:      "private",                                     // Maybe keep. Likely change. The default is possibly "host".
			// UsernsMode:   "private",                                     // Maybe keep. Possibly a more secure user namespace mode could be configured if we are tinkering with Docker Engine's daemon.json.
			// ShmSize:      0,                                             // Maybe keep. Likely change.
			// Sysctls:      nil,                                           // Maybe keep but change value. If it is about setting sysctl, the ones I saw weren't all that useful.
			// Runtime:      "",                                            // Maybe keep. It is probably about Docker runtimes like Kata. Maybe could be used to tighten security further.
			// Resources: container.Resources{
			// 	CPUShares: 768,                    // Relative to other containers. The default is likely 1024. Maybe keep. Maybe change.
			// 	Memory:    1 * 1024 * 1024 * 1024, // 1GB. Maybe keep. Maybe change.
			// 	NanoCPUs:  1 * 1000000000,         // 1 CPU. Maybe keep. Maybe change.

			// 	// Maybe add other.
			// },
			// MaskedPaths:   nil, // Maybe use.
			// ReadonlyPaths: nil, // Maybe use. It seems useful but not sure if I need it.
		},
		nil, // Maybe use &network.NetworkingConfig.
		nil, // Maybe use &v1.Platform.
		"",
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		// TODO: Remove the container.
	}()
	if len(createResp.Warnings) > 0 {
		slog.Warn("", "warnings", createResp.Warnings)
	}

	openInputTar, err := os.Open(".run/main.tar")
	if err != nil {
		panic(err)
	}
	err = cli.CopyToContainer(ctx, createResp.ID, "/user/run/input", openInputTar, container.CopyToContainerOptions{
		CopyUIDGID: false,
	})
	if err != nil {
		panic(err)
	}

	err = cli.ContainerStart(ctx, createResp.ID, container.StartOptions{})
	if err != nil {
		panic(err)
	}

	var waitResp container.WaitResponse
	waitRespCh, errCh := cli.ContainerWait(ctx, createResp.ID, container.WaitConditionNotRunning)
	select {
	case err = <-errCh:
		if err != nil {
			panic(err)
		}
	case waitResp = <-waitRespCh:
	case <-ctx.Done():
		panic(ctx.Err())
	}
	if waitResp.Error != nil {
		panic("waitResp.Error is not nil")
	}
	exitCode := int(waitResp.StatusCode)

	// TODO: Consider more container.LogOptions.
	// TODO: Do we need to close multiplexedLogReadCloser?
	logPipeReader, logPipeWriter := io.Pipe()
	multiplexedLogReadCloser, err := cli.ContainerLogs(ctx, createResp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		panic(err)
	}
	go func() {
		// TODO: Can we use a single writer for both?
		// TODO: Panics here will crash the entire process because this is a goroutine and it doesn't have a recover.
		_, err = stdcopy.StdCopy(logPipeWriter, logPipeWriter, multiplexedLogReadCloser)
		if err != nil {
			panic(err)
		}
		err = logPipeWriter.Close()
		if err != nil {
			panic(err)
		}
	}()

	// Upload log file from pipe to object storage.
	err = uploadFileContent(ctx, r.S3, *b.LogFileKey, logPipeReader)
	if err != nil {
		panic(err)
	}

	// If run succeeded, upload PDF file from SOMEWHERE to object storage.
	if exitCode == 0 {
		// COPY OUTPUT FILES FROM CONTAINER HERE.

		// FIXME: Use real output file reader.
		err = uploadFileContent(ctx, r.S3, *b.OutputFileKey, bytes.NewReader(nil))
		if err != nil {
			panic(err)
		}
	}

	err = cli.ContainerRemove(ctx, createResp.ID, container.RemoveOptions{}) // TODO: defer
	if err != nil {
		panic(err)
	}

	// Update build exit code.
	b, err = updateBuildExitCode(ctx, r.DB, b.ID, exitCode)
	if err != nil {
		return nil, fmt.Errorf("build.Runner: %w", err)
	}

	// Update build status to done.
	var doneStatus Status
	if exitCode == 0 {
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
