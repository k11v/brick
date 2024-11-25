package buildtask

import (
	"context"
)

var _ Broker = (*StubBroker)(nil)

type StubBroker struct{}

func (StubBroker) SendBuildTask(ctx context.Context, b *Build) error {
	return nil
}

func (StubBroker) ReceiveBuildTask(ctx context.Context) (*Build, error) {
	return &Build{}, nil
}
