package sqlstorage

type Table struct {
	Name             string
	Columns          []string
	DataField        string
	OnConflict       []string
	UpdateOnConflict []string
	// Optional condition for DO UPDATE; when false the conflicting row is left as is
	UpdateOnConflictWhere string
}

var MessageTable = Table{
	Name: "gows_messages",
	Columns: []string{
		"jid",
		"id",
		"timestamp",
		"from_me",
		"is_real",
		"data",
	},
	DataField: "data",
	OnConflict: []string{
		"id",
	},
	// is_real must be refreshed: a message first stored before its content was
	// decrypted is stamped is_real=false, and the re-upsert that brings the real
	// content only rewrote data, leaving the column stale. GetAllMessages filters
	// on the column, so such a message stayed invisible forever.
	UpdateOnConflict: []string{
		"timestamp",
		"data",
		"is_real",
	},
	// whatsmeow emits one event per enc child with the same id, so a key-only (SKDM) or other non-real event
	// can land after the real content - it must never replace a real stored message (#2289)
	UpdateOnConflictWhere: "NOT (gows_messages.is_real AND NOT EXCLUDED.is_real)",
}

var GroupTable = Table{
	Name: "gows_groups",
	Columns: []string{
		"id",
		"name",
		"data",
	},
	DataField: "data",
	OnConflict: []string{
		"id",
	},
	UpdateOnConflict: []string{
		"name",
		"data",
	},
}

var ChatEphemeralSettingsTable = Table{
	Name: "gows_chat_ephemeral_setting",
	Columns: []string{
		"id",
		"data",
	},
	DataField: "data",
	OnConflict: []string{
		"id",
	},
	UpdateOnConflict: []string{
		"data",
	},
}

var LabelsTable = Table{
	Name: "gows_labels",
	Columns: []string{
		"id",
		"data",
	},
	DataField: "data",
	OnConflict: []string{
		"id",
	},
	UpdateOnConflict: []string{
		"data",
	},
}

var LabelAssociationsTable = Table{
	Name: "gows_label_associations",
	Columns: []string{
		"jid",
		"label_id",
		"data",
	},
	DataField: "data",
	OnConflict: []string{
		"jid",
		"label_id",
	},
	UpdateOnConflict: []string{
		"data",
	},
}
