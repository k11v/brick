package build

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/textproto"
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

		// Create input untar container.
		inputUntarCont, err := cli.ContainerCreate(
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
					VolumeOptions: &mount.VolumeOptions{
						NoCopy: true,
					},
				}},
			},
			nil,
			nil,
			"",
		)
		if err != nil {
			return err
		}
		defer func() {
			err = cli.ContainerRemove(ctx, inputUntarCont.ID, container.RemoveOptions{})
			if err != nil {
				slog.Error("didn't remove container", "id", inputUntarCont.ID, "error", err)
			}
		}()

		createResp, err := cli.ContainerCreate(
			ctx,
			&container.Config{
				Image:        "brick-runner",
				Entrypoint:   strslice.StrSlice{"/bin/sh", "-c", `mkdir /user/mnt/input && cd /user/mnt/input && exec runner "$@"`, "_"},
				Cmd:          strslice.StrSlice{"-i", "main.md", "-o", "/user/mnt/output/main.pdf", "-c", "/user/mnt/output/cache"},
				AttachStdin:  true,
				AttachStderr: true,
				AttachStdout: true,
				OpenStdin:    true,
				StdinOnce:    true,
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
				ReadonlyRootfs: false, // TODO: Consider true.
				Mounts: []mount.Mount{{
					Type:   mount.TypeTmpfs,
					Target: "/user/mnt",
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
				// Tmpfs:        map[string]string{"/user/mnt": "size=256m"}, // Maybe keep. Maybe change.
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

		attachResp, err := cli.ContainerAttach(ctx, createResp.ID, container.AttachOptions{
			Stream:     true,
			Stdin:      true,
			Stdout:     false, // TODO: Consider using instead of ContainerLogs.
			Stderr:     false, // TODO: Consider using instead of ContainerLogs.
			Logs:       false, // TODO: Consider using instead of ContainerLogs.
			DetachKeys: "",    // TODO: Consider what happens when stdin I pass contains default detach keys.
		})
		if err != nil {
			panic(err)
		}
		defer attachResp.Close()

		err = cli.ContainerStart(ctx, createResp.ID, container.StartOptions{})
		if err != nil {
			panic(err)
		}

		mw := multipart.NewWriter(attachResp.Conn)
		tarStdinWriter, err := mw.CreatePart(textproto.MIMEHeader{})
		if err != nil {
			panic(err)
		}
		_, err = io.Copy(tarStdinWriter, inputTarReader)
		if err != nil {
			panic(err)
		}
		containerRunnerParams := struct {
			InputFile string `json:"input_file"`
		}{
			InputFile: "main.md",
		}
		paramsStdinWriter, err := mw.CreatePart(textproto.MIMEHeader{})
		if err != nil {
			panic(err)
		}
		enc := json.NewEncoder(paramsStdinWriter)
		err = enc.Encode(containerRunnerParams)
		if err != nil {
			panic(err)
		}
		err = mw.Close()
		if err != nil {
			panic(err)
		}
		err = attachResp.CloseWrite()
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

		// Collect stderr.
		// TODO: Consider more container.LogOptions.
		// TODO: Do we need to close multiplexedLogReadCloser?
		stderrReader, stderrPipeWriter := io.Pipe()
		attachStderrResp, err := cli.ContainerAttach(ctx, createResp.ID, container.AttachOptions{
			Stderr: true,
			Logs:   true,
		})
		if err != nil {
			panic(err)
		}
		defer attachStderrResp.Close()
		go func() {
			// TODO: Can we use a single writer for both?
			// TODO: Panics here will crash the entire process because this is a goroutine and it doesn't have a recover.
			_, err = stdcopy.StdCopy(io.Discard, stderrPipeWriter, attachStderrResp.Reader)
			if err != nil {
				panic(err)
			}
			err = stderrPipeWriter.Close()
			if err != nil {
				panic(err)
			}
		}()
		stderrBytes, err := io.ReadAll(stderrReader) // TODO: stderr shouldn't be long but maybe some kind of limit should be in place.
		if err != nil {
			panic(err)
		}
		stderrString := string(stderrBytes)
		if stderrString != "" {
			slog.Warn("non-empty runner stderr", "stderr", stderrString)
		}
		if waitResp.StatusCode != 0 {
			panic("waitResp.StatusCode is not 0") // TODO: Log stderr.
		}

		stdoutReader, stdoutPipeWriter := io.Pipe()
		attachStdoutResp, err := cli.ContainerAttach(ctx, createResp.ID, container.AttachOptions{
			Stdout: true,
			Logs:   true,
		})
		if err != nil {
			panic(err)
		}
		defer attachStdoutResp.Close()
		go func() {
			// TODO: Can we use a single writer for both?
			// TODO: Panics here will crash the entire process because this is a goroutine and it doesn't have a recover.
			_, err = stdcopy.StdCopy(stdoutPipeWriter, io.Discard, attachStdoutResp.Reader)
			if err != nil {
				panic(err)
			}
			err = stdoutPipeWriter.Close()
			if err != nil {
				panic(err)
			}
		}()

		// Peek and detect the multipart boundary.
		// The boundary line should be less than 74 bytes:
		// 2 bytes for "--", up to 70 bytes for user-defined boundary, and 2 bytes for "\r\n".
		// See https://datatracker.ietf.org/doc/html/rfc1341.
		bufstdoutReader := bufio.NewReader(stdoutReader)
		peek, err := bufstdoutReader.Peek(74)
		if err != nil && err != io.EOF {
			panic(fmt.Errorf("invalid stdin boundary: %w", err))
		}
		boundary := string(peek)
		if boundary[:2] != "--" {
			panic(errors.New("invalid stdin boundary start"))
		}
		boundaryEnd := strings.Index(boundary, "\r\n")
		if boundaryEnd == -1 {
			panic(errors.New("invalid stdin boundary length or end"))
		}
		boundary = boundary[2:boundaryEnd]

		mr := multipart.NewReader(bufstdoutReader, boundary)

		// Upload log file from pipe to object storage.
		logFileReader, err := mr.NextPart()
		if err != nil {
			panic(err)
		}
		err = uploadFileContent(ctx, r.S3, *b.LogFileKey, logFileReader)
		if err != nil {
			panic(err)
		}

		// Get container runner result.
		var result struct {
			ExitCode int `json:"exit_code"`
		}
		resultReader, err := mr.NextPart()
		if err != nil {
			panic(err)
		}
		dec := json.NewDecoder(resultReader)
		dec.DisallowUnknownFields()
		err = dec.Decode(&result)
		if err != nil {
			panic(err)
		}
		if dec.More() {
			panic("multiple top-level elements")
		}

		// Upload output file from pipe to object storage.
		outputFileReader, err := mr.NextPart()
		if err != nil {
			panic(err)
		}
		err = uploadFileContent(ctx, r.S3, *b.OutputFileKey, outputFileReader)
		if err != nil {
			panic(err)
		}

		err = cli.ContainerRemove(ctx, createResp.ID, container.RemoveOptions{}) // TODO: defer
		if err != nil {
			panic(err)
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
