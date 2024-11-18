package operation

var _ Broker = (*StubBroker)(nil)

type StubBroker struct{}
