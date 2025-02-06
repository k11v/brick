package build

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"

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

	"github.com/k11v/brick/internal/app"
)

var ErrNotFound = errors.New("not found")

type ExitError struct {
	ExitCode int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code is %d", e.ExitCode)
}

type Doer struct {
	DB  *pgxpool.Pool
	STG *s3.Client
}

func NewDoer(db *pgxpool.Pool, stg *s3.Client) *Doer {
	return &Doer{
		DB:  db,
		STG: stg,
	}
}

type DoerDoParams struct {
	ID uuid.UUID
}

func (r *Doer) Do(ctx context.Context, params *DoerDoParams) (*Build, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Get build for status update to doing.
	b, err := getForUpdate(ctx, r.DB, params.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	// If build is being done, return.
	if b.Status == StatusDoing {
		return nil, fmt.Errorf("build.Doer: %w", ErrAlreadyDoing)
	}

	// If build is done, return.
	if b.Status == StatusDone {
		return nil, fmt.Errorf("build.Doer: %w", ErrAlreadyDone)
	}

	// Update build status to doing.
	b, err = updateStatus(ctx, tx, params.ID, StatusDoing, "")
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	// Get build files.
	files, err := getFiles(ctx, r.DB, b.ID)
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	// Prepare reader with input tar.
	inputTarReader, inputTarWriter := io.Pipe()
	defer inputTarReader.Close()
	inputTarErrCh := make(chan error, 1)
	go func() {
		defer close(inputTarErrCh)
		defer func() {
			err := inputTarWriter.Close()
			if err != nil {
				slog.Error("didn't close inputTarWriter", "error", err)
			}
		}()

		tw := tar.NewWriter(inputTarWriter)
		defer func() {
			err := tw.Close()
			if err != nil {
				slog.Error("didn't close tar.Writer", "error", err)
			}
		}()
		dirExist := make(map[string]struct{})

		for _, buildInputFile := range files {
			dir := buildInputFile.Name
			for {
				nextDir := path.Dir(dir)
				if nextDir == dir {
					break
				}
				dir = nextDir

				if _, exist := dirExist[dir]; !exist {
					err = tw.WriteHeader(&tar.Header{
						Typeflag: tar.TypeDir,
						Name:     dir,
						Mode:     0o777, // TODO: Check mode.
					})
					if err != nil {
						inputTarErrCh <- err
						return
					}
					dirExist[dir] = struct{}{}
				}
			}

			var buf bytes.Buffer
			err = downloadData(ctx, r.STG, &buf, buildInputFile.DataKey)
			if err != nil {
				inputTarErrCh <- err
				return
			}
			err = tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeReg,
				Name:     buildInputFile.Name,
				Mode:     0o666, // TODO: Check mode.
				Size:     int64(buf.Len()),
			})
			if err != nil {
				inputTarErrCh <- err
				return
			}
			_, err = tw.Write(buf.Bytes())
			if err != nil {
				inputTarErrCh <- err
				return
			}
		}
	}()

	// Do.
	err = func() error {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return err
		}

		// Create log writer that uploads to object storage.
		uploadLogDone := make(chan struct{})
		defer func() {
			<-uploadLogDone
		}()
		logReader, logWriter := io.Pipe()
		defer func() {
			err := logWriter.Close()
			if err != nil {
				slog.Error("didn't close logWriter", "error", err)
			}
		}()
		go func() {
			defer close(uploadLogDone)
			err := uploadFileData(ctx, r.STG, b.LogDataKey, logReader)
			if err != nil {
				_ = logReader.CloseWithError(err) // TODO: Check if used correctly.
				return
			}
		}()

		// Create volume.
		vol, err := cli.VolumeCreate(ctx, volume.CreateOptions{})
		if err != nil {
			return err
		}
		defer func() {
			err := cli.VolumeRemove(ctx, vol.Name, false)
			if err != nil {
				slog.Error("didn't remove volume", "id", vol.Name, "error", err)
			}
		}()

		// Run untar input container.
		err = func() error {
			// Create untar input container.
			untarInputCont, err := cli.ContainerCreate(
				ctx,
				&container.Config{
					Image:      "brick-build",
					Entrypoint: strslice.StrSlice{},
					Cmd: strslice.StrSlice{
						"sh",
						"-c",
						`
							set -e
							mkdir /user/input
							cd /user/input
							exec tar -v -x
						`,
					},
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
						Target: "/user",
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

			// Attach untar input container streams.
			untarInputContConn, err := cli.ContainerAttach(ctx, untarInputCont.ID, container.AttachOptions{
				Stream:     true,
				Stdin:      true,
				Stdout:     true,
				Stderr:     true,
				DetachKeys: "", // TODO: Consider DetachKeys.
			})
			if err != nil {
				return err
			}
			defer untarInputContConn.Close()

			// Start untar input container.
			err = cli.ContainerStart(ctx, untarInputCont.ID, container.StartOptions{})
			if err != nil {
				return err
			}

			// Write untar input container stdin.
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

			// Read untar input container stdout and stderr.
			_, err = logWriter.Write([]byte("$ untar\n"))
			if err != nil {
				return err
			}
			_, err = stdcopy.StdCopy(logWriter, logWriter, untarInputContConn.Conn)
			if err != nil {
				return err
			}

			// Check untar input container stdin error.
			err = <-stdinErrCh
			if err != nil {
				return err
			}

			// Check untar input container exit code.
			untarInputContInspect, err := cli.ContainerInspect(ctx, untarInputCont.ID)
			if untarInputContInspect.State.Status != "exited" {
				return errors.New("didn't exit")
			}
			if untarInputContInspect.State.ExitCode != 0 {
				return &ExitError{ExitCode: untarInputContInspect.State.ExitCode}
			}

			return nil
		}()
		if err != nil {
			return err
		}

		// Run build container.
		err = func() error {
			// Create build container.
			buildCont, err := cli.ContainerCreate(
				ctx,
				&container.Config{
					Image:      "brick-build",
					Entrypoint: strslice.StrSlice{},
					Cmd: strslice.StrSlice{
						"sh",
						"-c",
						`
							set -e
							cd /user/input
							mkdir /user/output
							exec build -i main.md -o /user/output/main.pdf -c /user/cache
						`,
					},
					AttachStdout: true,
					AttachStderr: true,
				},
				&container.HostConfig{
					NetworkMode:    "none",
					CapDrop:        strslice.StrSlice{"ALL"},
					CapAdd:         strslice.StrSlice{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FSETID", "CAP_FOWNER", "CAP_MKNOD", "CAP_NET_RAW", "CAP_SETGID", "CAP_SETUID", "CAP_SETFCAP", "CAP_SETPCAP", "CAP_NET_BIND_SERVICE", "CAP_SYS_CHROOT", "CAP_KILL", "CAP_AUDIT_WRITE"},
					ReadonlyRootfs: true,
					Mounts: []mount.Mount{{
						Type:   mount.TypeVolume,
						Source: vol.Name,
						Target: "/user",
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
				err = cli.ContainerRemove(ctx, buildCont.ID, container.RemoveOptions{})
				if err != nil {
					slog.Error("didn't remove container", "id", buildCont.ID, "error", err)
				}
			}()

			// Attach build container streams.
			buildContConn, err := cli.ContainerAttach(ctx, buildCont.ID, container.AttachOptions{
				Stream: true,
				Stdout: true,
				Stderr: true,
			})
			if err != nil {
				return err
			}
			defer buildContConn.Close()

			// Start build container.
			err = cli.ContainerStart(ctx, buildCont.ID, container.StartOptions{})
			if err != nil {
				return err
			}

			// Read build container stdout and stderr.
			_, err = logWriter.Write([]byte("$ build\n"))
			if err != nil {
				return err
			}
			_, err = stdcopy.StdCopy(logWriter, logWriter, buildContConn.Conn)
			if err != nil {
				return err
			}

			// Check build container exit code.
			buildContInspect, err := cli.ContainerInspect(ctx, buildCont.ID)
			if buildContInspect.State.Status != "exited" {
				return errors.New("didn't exit")
			}
			if buildContInspect.State.ExitCode != 0 {
				return &ExitError{ExitCode: buildContInspect.State.ExitCode}
			}

			return nil
		}()
		if err != nil {
			return err
		}

		// Run cat output container.
		err = func() error {
			// Create cat output container.
			catOutputCont, err := cli.ContainerCreate(
				ctx,
				&container.Config{
					Image:      "brick-build",
					Entrypoint: strslice.StrSlice{},
					Cmd: strslice.StrSlice{
						"sh",
						"-c",
						`
							set -e
							cd /user/output
							exec cat main.pdf
						`,
					},
					AttachStdout: true,
					AttachStderr: true,
				},
				&container.HostConfig{
					NetworkMode:    "none",
					CapDrop:        strslice.StrSlice{"ALL"},
					CapAdd:         strslice.StrSlice{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FSETID", "CAP_FOWNER", "CAP_MKNOD", "CAP_NET_RAW", "CAP_SETGID", "CAP_SETUID", "CAP_SETFCAP", "CAP_SETPCAP", "CAP_NET_BIND_SERVICE", "CAP_SYS_CHROOT", "CAP_KILL", "CAP_AUDIT_WRITE"},
					ReadonlyRootfs: true,
					Mounts: []mount.Mount{{
						Type:   mount.TypeVolume,
						Source: vol.Name,
						Target: "/user",
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
				err = cli.ContainerRemove(ctx, catOutputCont.ID, container.RemoveOptions{})
				if err != nil {
					slog.Error("didn't remove container", "id", catOutputCont.ID, "error", err)
				}
			}()

			// Attach cat output container streams.
			catOutputContConn, err := cli.ContainerAttach(ctx, catOutputCont.ID, container.AttachOptions{
				Stream:     true,
				Stdout:     true,
				Stderr:     true,
				DetachKeys: "", // TODO: Consider DetachKeys.
			})
			if err != nil {
				return err
			}
			defer catOutputContConn.Close()

			// Start cat output container.
			err = cli.ContainerStart(ctx, catOutputCont.ID, container.StartOptions{})
			if err != nil {
				return err
			}

			// Create output file writer that uploads to object storage.
			uploadOutputFileDone := make(chan struct{})
			defer func() {
				<-uploadOutputFileDone
			}()
			outputFileReader, outputFileWriter := io.Pipe()
			defer func() {
				err := outputFileWriter.Close()
				if err != nil {
					slog.Error("didn't close outputFileWriter", "error", err)
				}
			}()
			go func() {
				defer close(uploadOutputFileDone)
				err := uploadFileData(ctx, r.STG, b.OutputDataKey, outputFileReader)
				if err != nil {
					_ = outputFileReader.CloseWithError(err) // TODO: Check if used correctly.
					return
				}
			}()

			// Read cat output container stdout and stderr.
			_, err = logWriter.Write([]byte("$ cat\n"))
			if err != nil {
				return err
			}
			_, err = stdcopy.StdCopy(outputFileWriter, logWriter, catOutputContConn.Conn)
			if err != nil {
				return err
			}

			// Check cat output container exit code.
			catOutputContInspect, err := cli.ContainerInspect(ctx, catOutputCont.ID)
			if catOutputContInspect.State.Status != "exited" {
				return errors.New("didn't exit")
			}
			if catOutputContInspect.State.ExitCode != 0 {
				return &ExitError{ExitCode: catOutputContInspect.State.ExitCode}
			}

			return nil
		}()
		if err != nil {
			return err
		}

		return nil
	}()
	exitCode := 0
	if exitErr := (*ExitError)(nil); errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	err = <-inputTarErrCh
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	// Update build exit code.
	b, err = updateExitCode(ctx, r.DB, b.ID, exitCode)
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	// Update build status to done.
	var errorValue Error
	if exitCode != 0 {
		errorValue = ErrorExitedWithNonZero
	}
	b, err = updateStatus(ctx, r.DB, b.ID, StatusDone, errorValue)
	if err != nil {
		return nil, fmt.Errorf("build.Doer: %w", err)
	}

	return b, nil
}

func getBuild(ctx context.Context, db executor, id uuid.UUID) (*Build, error) {
	query := `
		SELECT id, created_at, idempotency_key, user_id, status, error, exit_code, log_data_key, output_data_key
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

func getFiles(ctx context.Context, db executor, buildID uuid.UUID) ([]*File, error) {
	query := `
		SELECT id, build_id, name, type, data_key
		FROM build_files
		WHERE build_id = $1
		ORDER BY name, build_id
	`
	args := []any{buildID}

	rows, _ := db.Query(ctx, query, args...)
	files, err := pgx.CollectRows(rows, rowToFile)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// downloadPartSize should be greater than or equal 5MB.
// See github.com/aws/aws-sdk-go-v2/feature/s3/manager.
const downloadPartSize = 10 * 1024 * 1024 // 10MB

func downloadData(ctx context.Context, s3Client *s3.Client, w io.Writer, key string) error {
	downloader := manager.NewDownloader(s3Client, func(d *manager.Downloader) {
		d.PartSize = int64(downloadPartSize)
		d.Concurrency = 1
	})

	// fakeWriterAt needs manager.Downloader.Concurrency set to 1.
	_, err := downloader.Download(ctx, fakeWriterAt{w}, &s3.GetObjectInput{
		Bucket: &app.S3BucketName,
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

// If exitCode is negative, updateExitCode sets it to NULL.
func updateExitCode(ctx context.Context, db executor, id uuid.UUID, exitCode int) (*Build, error) {
	var exitCodeArg *int
	if exitCode >= 0 {
		exitCodeArg = new(int)
		*exitCodeArg = exitCode
	}

	query := `
		UPDATE builds
		SET exit_code = $2
		WHERE id = $1
		RETURNING id, created_at, idempotency_key, user_id, status, error, exit_code, log_data_key, output_data_key
	`
	args := []any{id, exitCodeArg}

	rows, _ := db.Query(ctx, query, args...)
	b, err := pgx.CollectExactlyOneRow(rows, rowToBuild)
	if err != nil {
		return nil, err
	}

	return b, nil
}
