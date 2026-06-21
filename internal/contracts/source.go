package contracts

import "context"

type ProviderRef struct {
	Type string
	Name string
}

type SourceObjectRef struct {
	Provider    ProviderRef
	ID          string
	DisplayName string
}

type DesiredRecord struct {
	Hostname string
	Answer   string
	Source   SourceObjectRef
	Output   string
}

type SourceWatch struct {
	Hints <-chan struct{}
	Err   <-chan error
}

type Source interface {
	Provider() ProviderRef
	ListDesired(context.Context) ([]DesiredRecord, error)
}

type WatchableSource interface {
	Watch(context.Context) SourceWatch
}
