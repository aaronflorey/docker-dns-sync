package contracts

import "context"

type RecordProvenance struct {
	RemoteID string
}

type VisibleRecord struct {
	Output     ProviderRef
	Hostname   string
	Answer     string
	Provenance *RecordProvenance
}

type Output interface {
	Provider() ProviderRef
	ListVisible(context.Context) ([]VisibleRecord, error)
	Create(context.Context, DesiredRecord) (*RecordProvenance, error)
	Update(context.Context, VisibleRecord, DesiredRecord) (*RecordProvenance, error)
	Delete(context.Context, VisibleRecord) error
}
