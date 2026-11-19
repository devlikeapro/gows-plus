package sqlstorage

import (
	"encoding/json"
	"github.com/devlikeapro/gows/storage"
)

type CallLogMapper struct {
}

var _ Mapper[storage.CallLog] = (*CallLogMapper)(nil)
var callLogMapper = &CallLogMapper{}

func (f *CallLogMapper) ToFields(entity *storage.CallLog) map[string]interface{} {
	return map[string]interface{}{
		"id":        entity.ID,
		"jid":       entity.Chat.String(),
		"timestamp": entity.Timestamp.Unix(),
		"from_me":   entity.FromMe,
	}
}

func (f *CallLogMapper) Marshal(log *storage.CallLog) ([]byte, error) {
	return json.Marshal(log)
}

func (f *CallLogMapper) Unmarshal(data []byte, log *storage.CallLog) error {
	return json.Unmarshal(data, log)
}
