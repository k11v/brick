package operation

import (
	"context"

	"github.com/k11v/brick/internal/app/build"
)

type Broker interface {
	SendBuildTask(ctx context.Context, b *build.Build) error
	ReceiveBuildTask(ctx context.Context) (*build.Build, error)
}
