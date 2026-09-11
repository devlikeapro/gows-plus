package sqlstorage

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/avast/retry-go"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
)

type SqlMessageStore struct {
	*EntityRepository[storage.StoredMessage]
}

var _ storage.MessageStorage = (*SqlMessageStore)(nil)

func (gc *GContainer) NewMessageStorage() *SqlMessageStore {
	repo := NewEntityRepository[storage.StoredMessage](
		gc.db,
		MessageTable,
		messageMapper,
	)
	return &SqlMessageStore{
		repo,
	}
}

func (s SqlMessageStore) UpsertOneMessage(msg *storage.StoredMessage) (err error) {
	return s.UpsertOne(msg)
}

func (s SqlMessageStore) GetAllMessages(filters storage.MessageFilter, sort storage.Sort, pagination storage.Pagination, merge bool) ([]*storage.StoredMessage, error) {
	conditions := make([]sq.Sqlizer, 0)
	if filters.Jid != nil {
		target := *filters.Jid
		jidColumn := fmt.Sprintf("%s.jid", s.table.Name)
		if merge {
			var err error
			target, err = s.canonicalizeJID(*filters.Jid)
			if err != nil {
				return nil, err
			}
			// Resolve all LID JIDs that map to this phone number in one pre-lookup,
			// so the WHERE clause uses exact equality and the (jid, timestamp) index.
			lids, err := s.reverseLookupLIDs(target.User)
			if err != nil {
				return nil, err
			}
			jidStrs := append([]string{target.String()}, lids...)
			expr, args := buildInExpression(jidColumn, jidStrs)
			conditions = append(conditions, sq.Expr(expr, args...))
		} else {
			conditions = append(conditions, sq.Eq{jidColumn: target.String()})
		}
	}
	if filters.TimestampGte != nil {
		conditions = append(conditions, sq.GtOrEq{"timestamp": filters.TimestampGte})
	}
	if filters.TimestampLte != nil {
		conditions = append(conditions, sq.LtOrEq{"timestamp": filters.TimestampLte})
	}
	if filters.FromMe != nil {
		conditions = append(conditions, sq.Eq{"from_me": filters.FromMe})
	}
	if filters.Status != nil {
		switch s.db.DriverName() {
		case "sqlite3":
			conditions = append(conditions, sq.Expr("json_extract(data, '$.Status') = ?", *filters.Status))
		case "postgres":
			conditions = append(conditions, sq.Expr("(data::jsonb->>'Status')::int = ?", *filters.Status))
		default:
			return nil, fmt.Errorf("unsupported database driver: %s", s.db.DriverName())
		}
	}

	conditions = append(conditions, sq.Eq{"is_real": true})
	sorts := []storage.Sort{sort}
	return s.FilterBy(conditions, sorts, pagination)
}

func (s SqlMessageStore) GetChatMessages(jid types.JID, filters storage.MessageFilter, pagination storage.Pagination, merge bool) ([]*storage.StoredMessage, error) {
	filters.Jid = &jid
	sort := storage.Sort{
		Field: "timestamp",
		Order: storage.SortDesc,
	}
	return s.GetAllMessages(filters, sort, pagination, merge)
}

func (s SqlMessageStore) GetMessage(id types.MessageID) (msg *storage.StoredMessage, err error) {
	return s.GetById(id)
}

func (s SqlMessageStore) GetMessageWithRetries(id types.MessageID) (msg *storage.StoredMessage, err error) {
	err = retry.Do(
		func() error {
			msg, err = s.GetById(id)
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(6),
		// Return the last error as-is so errors.Is(err, storage.ErrNotFound) works
		retry.LastErrorOnly(true),
	)
	return msg, err
}

func (s SqlMessageStore) DeleteChatMessages(jid types.JID, deleteBefore time.Time) error {
	conditions := []sq.Sqlizer{
		sq.Eq{"jid": jid},
		sq.Lt{"timestamp": deleteBefore},
	}
	return s.DeleteBy(conditions)
}

func (s SqlMessageStore) DeleteMessage(id types.MessageID) error {
	return s.DeleteById(id)
}

// getLastMessagesPostgresSubquery generates the subquery for PostgreSQL to fetch the ID of the last message per chat.
func (s SqlMessageStore) getLastMessagesPostgresSubquery(primaryExpr string, priorityExpr string) *sq.SelectBuilder {
	query := sq.Select("DISTINCT ON (" + primaryExpr + ") id").
		From(s.table.Name).
		Where("is_real = true").
		OrderByClause(primaryExpr).
		OrderByClause("timestamp DESC")
	if priorityExpr != "" {
		query = query.OrderByClause(priorityExpr)
	}
	return &query
}

// getLastMessagesSQLiteSubquery generates the subquery for SQLite3 to fetch the ID of the last message per chat.
func (s SqlMessageStore) getLastMessagesSQLiteSubquery(primaryExpr string, priorityExpr string) *sq.SelectBuilder {
	ordering := "timestamp DESC"
	if priorityExpr != "" {
		ordering = fmt.Sprintf("%s, %s", ordering, priorityExpr)
	}
	query := sq.Select("id").
		FromSelect(
			sq.Select(
				"id",
				"jid",
				"timestamp",
				"ROW_NUMBER() OVER (PARTITION BY ("+primaryExpr+") ORDER BY "+ordering+") as rn",
			).
				From(s.table.Name).
				Where("is_real = true"),
			"sub").
		Where("rn = 1")
	return &query
}

// getLastMessageSubquery selects the appropriate subquery based on the database type.
func (s SqlMessageStore) getLastMessageSubquery(primaryExpr string, priorityExpr string) (*sq.SelectBuilder, error) {
	switch s.db.DriverName() {
	case "postgres":
		return s.getLastMessagesPostgresSubquery(primaryExpr, priorityExpr), nil
	case "sqlite3":
		return s.getLastMessagesSQLiteSubquery(primaryExpr, priorityExpr), nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", s.db.DriverName())
	}
}

// GetLastMessagesInChats retrieves the last messages in chats based on filtering, sorting, and pagination.
func (s SqlMessageStore) GetLastMessagesInChats(filter storage.ChatFilter, sortBy storage.Sort, pagination storage.Pagination, merge bool) ([]*storage.StoredMessage, error) {
	// When merging, replace the CASE WHEN correlated-subquery approach with two
	// separate index-friendly queries (non-lid / lid) merged in memory by timestamp.
	if merge {
		return s.getLastMessagesInChatsWithMerge(filter, sortBy, pagination)
	}

	// merge=false: simple DISTINCT ON (jid) with no CASE expression — index-friendly as-is
	primaryExpr := fmt.Sprintf("%s.jid", s.table.Name)
	subQuery, err := s.getLastMessageSubquery(primaryExpr, "")
	if err != nil {
		return nil, err
	}
	subQueryText, _, err := (*subQuery).ToSql()
	if err != nil {
		return nil, err
	}

	sql := sq.Select("data").
		From(s.table.Name).
		Where("id IN (" + subQueryText + ")")
	if len(filter.Jids) > 0 {
		jidStrs, err := s.targetJIDStrings(filter.Jids, false)
		if err != nil {
			return nil, err
		}
		expr, args := buildInExpression(fmt.Sprintf("%s.jid", s.table.Name), jidStrs)
		sql = sql.Where(sq.Expr(expr, args...))
	}

	return s.Retrieve(sql, pagination, []storage.Sort{sortBy})
}

// getLastMessagesInChatsWithMerge replaces the CASE WHEN correlated-subquery approach
// with two separate queries (non-lid and lid) merged in memory by timestamp.
// Each subquery uses the (jid, timestamp) index directly with no per-row subqueries.
// Works for both PostgreSQL (DISTINCT ON) and SQLite (ROW_NUMBER window function).
func (s SqlMessageStore) getLastMessagesInChatsWithMerge(filter storage.ChatFilter, sortBy storage.Sort, pagination storage.Pagination) ([]*storage.StoredMessage, error) {
	// When a JID filter is provided, expand each JID to its canonical form + LID variants
	// so each subquery gets a targeted jid IN (...) filter on the raw column.
	var nonLidJIDFilter []string
	var lidJIDFilter []string
	hasJIDFilter := len(filter.Jids) > 0

	if hasJIDFilter {
		for _, jid := range filter.Jids {
			canonical, err := s.canonicalizeJID(jid)
			if err != nil {
				return nil, err
			}
			nonLidJIDFilter = append(nonLidJIDFilter, canonical.String())
			lids, err := s.reverseLookupLIDs(canonical.User)
			if err != nil {
				return nil, err
			}
			lidJIDFilter = append(lidJIDFilter, lids...)
		}
	}

	// Query 1: latest non-@lid message per chat — uses (jid, timestamp) index efficiently
	nonLidMessages, err := s.fetchDistinctLatestByJID("jid NOT LIKE '%@lid'", nonLidJIDFilter)
	if err != nil {
		return nil, err
	}

	// Query 2: latest @lid message per chat — only when relevant (small set)
	var lidMessages []*storage.StoredMessage
	if !hasJIDFilter || len(lidJIDFilter) > 0 {
		lidMessages, err = s.fetchDistinctLatestByJID("jid LIKE '%@lid'", lidJIDFilter)
		if err != nil {
			return nil, err
		}
		// Canonicalize @lid → @s.whatsapp.net so timestamps can be compared with non-lid results
		for _, msg := range lidMessages {
			msg.Info.Chat, err = s.canonicalizeJID(msg.Info.Chat)
			if err != nil {
				return nil, err
			}
		}
	}

	// Merge: per canonical JID keep whichever message has the later timestamp
	merged := mergeLastMessages(nonLidMessages, lidMessages)

	// Sort in memory (mirrors what the DB ORDER BY would have produced)
	sort.Slice(merged, func(i, j int) bool {
		if sortBy.Order == storage.SortDesc {
			return merged[i].Info.Timestamp.After(merged[j].Info.Timestamp)
		}
		return merged[i].Info.Timestamp.Before(merged[j].Info.Timestamp)
	})

	// Paginate in memory
	start := int(pagination.Offset)
	if start >= len(merged) {
		return nil, nil
	}
	end := len(merged)
	if pagination.Limit > 0 {
		end = start + int(pagination.Limit)
		if end > len(merged) {
			end = len(merged)
		}
	}
	return merged[start:end], nil
}

// fetchDistinctLatestByJID gets the latest message ID per chat matching jidCondition,
// optionally filtered to jidFilter values, then fetches full message data by primary key.
// Uses DISTINCT ON for PostgreSQL and ROW_NUMBER() OVER (PARTITION BY jid) for SQLite.
func (s SqlMessageStore) fetchDistinctLatestByJID(jidCondition string, jidFilter []string) ([]*storage.StoredMessage, error) {
	var sub sq.SelectBuilder
	switch s.db.DriverName() {
	case "postgres":
		sub = sq.Select("DISTINCT ON (jid) id").
			From(s.table.Name).
			Where("is_real = true").
			Where(jidCondition).
			OrderByClause("jid").
			OrderByClause("timestamp DESC")
	case "sqlite3":
		sub = sq.Select("id").
			FromSelect(
				sq.Select("id", "jid", "ROW_NUMBER() OVER (PARTITION BY jid ORDER BY timestamp DESC) as rn").
					From(s.table.Name).
					Where("is_real = true").
					Where(jidCondition),
				"sub").
			Where("rn = 1")
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", s.db.DriverName())
	}
	if len(jidFilter) > 0 {
		expr, args := buildInExpression("jid", jidFilter)
		sub = sub.Where(sq.Expr(expr, args...))
	}
	subSQL, subArgs, err := sub.ToSql()
	if err != nil {
		return nil, err
	}

	// Execute the subquery first to get IDs, then fetch data by primary key.
	// This avoids embedding a parameterized subquery inside another parameterized query.
	var ids []string
	if err := s.db.Select(&ids, subSQL, subArgs...); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	expr, args := buildInExpression("id", ids)
	mainQuery := sq.Select("data").
		From(s.table.Name).
		Where(sq.Expr(expr, args...))
	return s.Retrieve(mainQuery, storage.Pagination{}, nil)
}

// mergeLastMessages combines two message slices keyed by canonical chat JID,
// keeping whichever entry has the later timestamp when both sets contain the same chat.
func mergeLastMessages(primary, secondary []*storage.StoredMessage) []*storage.StoredMessage {
	byJID := make(map[string]*storage.StoredMessage, len(primary))
	for _, msg := range primary {
		byJID[msg.Info.Chat.String()] = msg
	}
	for _, msg := range secondary {
		key := msg.Info.Chat.String()
		if existing, ok := byJID[key]; !ok || msg.Info.Timestamp.After(existing.Info.Timestamp) {
			byJID[key] = msg
		}
	}
	result := make([]*storage.StoredMessage, 0, len(byJID))
	for _, msg := range byJID {
		result = append(result, msg)
	}
	return result
}

// expandJIDsWithLIDs returns canonical JID strings plus all known @lid JID strings
// for each JID in the input, deduplicated. Used to build a raw jid IN (...) filter
// that avoids CASE expressions on the indexed column.
func (s SqlMessageStore) expandJIDsWithLIDs(jids []types.JID) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string
	for _, jid := range jids {
		canonical, err := s.canonicalizeJID(jid)
		if err != nil {
			return nil, err
		}
		str := canonical.String()
		if _, ok := seen[str]; !ok {
			seen[str] = struct{}{}
			result = append(result, str)
		}
		lids, err := s.reverseLookupLIDs(canonical.User)
		if err != nil {
			return nil, err
		}
		for _, lid := range lids {
			if _, ok := seen[lid]; !ok {
				seen[lid] = struct{}{}
				result = append(result, lid)
			}
		}
	}
	return result, nil
}

func (s SqlMessageStore) canonicalizeJID(jid types.JID) (types.JID, error) {
	if jid.Server != types.HiddenUserServer {
		return jid, nil
	}
	query := sq.Select("pn").
		From("whatsmeow_lid_map").
		Where(sq.Eq{"lid": jid.User}).
		Limit(1)
	sqlText, args, err := query.ToSql()
	if err != nil {
		return types.JID{}, err
	}
	var pn string
	err = s.db.Get(&pn, sqlText, args...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return jid, nil
	case err != nil:
		return types.JID{}, err
	case pn == "":
		return jid, nil
	}
	canonical := types.JID{
		User:   pn,
		Server: types.DefaultUserServer,
		Device: jid.Device,
	}
	return canonical, nil
}

func (s SqlMessageStore) canonicalJIDStrings(jids []types.JID) ([]string, error) {
	result := make([]string, 0, len(jids))
	seen := make(map[string]struct{}, len(jids))
	for _, jid := range jids {
		canonical, err := s.canonicalizeJID(jid)
		if err != nil {
			return nil, err
		}
		str := canonical.String()
		if _, ok := seen[str]; ok {
			continue
		}
		seen[str] = struct{}{}
		result = append(result, str)
	}
	return result, nil
}

func (s SqlMessageStore) targetJIDStrings(jids []types.JID, merge bool) ([]string, error) {
	if merge {
		return s.canonicalJIDStrings(jids)
	}
	result := make([]string, 0, len(jids))
	seen := make(map[string]struct{}, len(jids))
	for _, jid := range jids {
		str := jid.String()
		if _, ok := seen[str]; ok {
			continue
		}
		seen[str] = struct{}{}
		result = append(result, str)
	}
	return result, nil
}

// reverseLookupLIDs returns all "@lid" JID strings that map to the given phone number.
// This enables building a WHERE jid IN (...) clause instead of a per-row correlated subquery.
func (s SqlMessageStore) reverseLookupLIDs(pn string) ([]string, error) {
	query, args, err := sq.Select("lid").
		From("whatsmeow_lid_map").
		Where(sq.Eq{"pn": pn}).
		ToSql()
	if err != nil {
		return nil, err
	}
	var lids []string
	if err := s.db.Select(&lids, query, args...); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(lids))
	for _, lid := range lids {
		result = append(result, lid+"@"+types.HiddenUserServer)
	}
	return result, nil
}

func (s SqlMessageStore) primaryJIDExpression(tableAlias string) (string, error) {
	column := fmt.Sprintf("%s.jid", tableAlias)
	userExpr, err := s.jidUserExpression(column)
	if err != nil {
		return "", err
	}
	pnLookup := fmt.Sprintf("(SELECT pn FROM whatsmeow_lid_map WHERE lid = %s LIMIT 1)", userExpr)
	pnJID := fmt.Sprintf("(%s || '@%s')", pnLookup, types.DefaultUserServer)
	expr := fmt.Sprintf("CASE WHEN %s LIKE '%%%%@lid' THEN COALESCE(%s, %s) ELSE %s END", column, pnJID, column, column)
	return expr, nil
}

func (s SqlMessageStore) primaryPriorityExpression(column string) string {
	return fmt.Sprintf("CASE WHEN %s LIKE '%%%%@lid' THEN 1 ELSE 0 END", column)
}

func (s SqlMessageStore) jidUserExpression(column string) (string, error) {
	switch s.db.DriverName() {
	case "postgres":
		return fmt.Sprintf("split_part(%s::text, '@', 1)", column), nil
	case "sqlite3":
		return fmt.Sprintf("substr(%s, 1, instr(%s, '@') - 1)", column, column), nil
	default:
		return "", fmt.Errorf("unsupported database driver: %s", s.db.DriverName())
	}
}

func buildInExpression(expr string, values []string) (string, []interface{}) {
	placeholders := make([]string, len(values))
	args := make([]interface{}, len(values))
	for i, value := range values {
		placeholders[i] = "?"
		args[i] = value
	}
	return fmt.Sprintf("%s IN (%s)", expr, strings.Join(placeholders, ",")), args
}
