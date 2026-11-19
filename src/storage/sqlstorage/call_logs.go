package sqlstorage

import (
	sq "github.com/Masterminds/squirrel"
	"github.com/devlikeapro/gows/storage"
)

// callLogUpsertChunk keeps one statement well below the sqlite bound-variable limit (5 columns per row)
const callLogUpsertChunk = 500

type SqlCallLogStore struct {
	*EntityRepository[storage.CallLog]
}

var _ storage.CallLogStorage = (*SqlCallLogStore)(nil)

func (gc *GContainer) NewCallLogStorage() *SqlCallLogStore {
	repo := NewEntityRepository[storage.CallLog](
		gc.db,
		CallLogsTable,
		callLogMapper,
	)
	return &SqlCallLogStore{
		repo,
	}
}

func (s SqlCallLogStore) UpsertCallLogs(logs []*storage.CallLog) error {
	// One statement must not touch the same row twice (postgres refuses it): the last entry for a call id wins
	index := make(map[string]int, len(logs))
	unique := make([]*storage.CallLog, 0, len(logs))
	for _, log := range logs {
		if log == nil || log.ID == "" {
			continue
		}
		if i, ok := index[log.ID]; ok {
			unique[i] = log
			continue
		}
		index[log.ID] = len(unique)
		unique = append(unique, log)
	}
	for start := 0; start < len(unique); start += callLogUpsertChunk {
		end := min(start+callLogUpsertChunk, len(unique))
		if err := s.UpsertMany(unique[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s SqlCallLogStore) GetCallLogs(filter storage.CallLogFilter, pagination storage.Pagination) ([]*storage.CallLog, error) {
	conditions := []sq.Sqlizer{}
	if filter.Jid != nil {
		conditions = append(conditions, sq.Eq{"jid": filter.Jid.String()})
	}
	sort := []storage.Sort{
		{Field: "timestamp", Order: storage.SortDesc},
		{Field: "id", Order: storage.SortDesc},
	}
	logs, err := s.FilterBy(conditions, sort, pagination)
	if err != nil {
		return nil, err
	}
	if logs == nil {
		logs = []*storage.CallLog{}
	}
	return logs, nil
}
