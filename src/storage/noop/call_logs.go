package noop

import (
	"github.com/devlikeapro/gows/storage"
)

type CallLogStorage struct{}

var _ storage.CallLogStorage = (*CallLogStorage)(nil)

func NewCallLogStorage() *CallLogStorage {
	return &CallLogStorage{}
}

func (s CallLogStorage) UpsertCallLogs(logs []*storage.CallLog) error {
	return nil
}

func (s CallLogStorage) GetCallLogs(filter storage.CallLogFilter, pagination storage.Pagination) ([]*storage.CallLog, error) {
	return []*storage.CallLog{}, nil
}
