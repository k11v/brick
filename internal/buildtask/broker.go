package buildtask

import (
	"context"
)

type Broker interface {
	SendBuildTask(ctx context.Context, b *Build) error
	ReceiveBuildTask(ctx context.Context) (*Build, error)
}
