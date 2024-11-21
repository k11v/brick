package buildtask

import "context"

var _ Storage = (*StubStorage)(nil)

type StubStorage struct{}

func (StubStorage) UploadFiles(context.Context, *StorageUploadFilesParams) error {
	return nil
}

func (StubStorage) DownloadFiles(context.Context, *StorageDownloadFilesParams) error {
	return nil
}
