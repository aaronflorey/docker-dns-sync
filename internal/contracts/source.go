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
}

type Source interface {
	Provider() ProviderRef
	ListDesired(context.Context) ([]DesiredRecord, error)
}
