package operation

import (
	"context"

	"github.com/k11v/brick/internal/build"
)

var _ Broker = (*StubBroker)(nil)

type StubBroker struct{}

func (StubBroker) SendBuildTask(ctx context.Context, b *build.Build) error {
	return nil
}

func (StubBroker) ReceiveBuildTask(ctx context.Context) (*build.Build, error) {
	return &build.Build{}, nil
}
