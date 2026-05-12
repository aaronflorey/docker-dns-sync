package contracts

import "context"

type VisibleRecord struct {
	Output   ProviderRef
	Hostname string
	Answer   string
}

type Output interface {
	Provider() ProviderRef
	ListVisible(context.Context) ([]VisibleRecord, error)
	Create(context.Context, DesiredRecord) error
	Update(context.Context, VisibleRecord, DesiredRecord) error
	Delete(context.Context, VisibleRecord) error
}
