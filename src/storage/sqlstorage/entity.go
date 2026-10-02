package sqlstorage

import (
	sq "github.com/Masterminds/squirrel"
	"github.com/devlikeapro/gows/storage"
	"github.com/jmoiron/sqlx"
	"strings"
)

func init() {
	// Postgres and SqLite use $1, $2, ... placeholders
	sq.StatementBuilder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
}

type Mapper[Entity any] interface {
	ToFields(*Entity) map[string]interface{}
	Marshal(*Entity) ([]byte, error)
	Unmarshal([]byte, *Entity) error
}

type EntityRepository[Entity any] struct {
	db         *sqlx.DB
	table      Table
	mapper     Mapper[Entity]
	onConflict string
}

func NewEntityRepository[Entity any](
	db *sqlx.DB,
	table Table,
	mapper Mapper[Entity],
) *EntityRepository[Entity] {
	return &EntityRepository[Entity]{
		db:         db,
		table:      table,
		mapper:     mapper,
		onConflict: onConflictDoUpdate(table.OnConflict, table.UpdateOnConflict, table.UpdateOnConflictWhere),
	}
}

func onConflictSetClause(fields []string) string {
	var set []string
	for _, f := range fields {
		set = append(set, f+" = EXCLUDED."+f)
	}
	return strings.Join(set, ", ")
}
func onConflictInner(fields []string) string {
	return strings.Join(fields, ", ")
}

func onConflictDoUpdate(conflictFields []string, setFields []string, where string) string {
	if len(conflictFields) == 0 {
		return ""
	}
	on := onConflictInner(conflictFields)
	set := onConflictSetClause(setFields)
	clause := "ON CONFLICT (" + on + ") DO UPDATE SET " + set
	if where != "" {
		clause += " WHERE " + where
	}
	return clause
}

func (kv *EntityRepository[Entity]) UpsertOne(entity *Entity) error {
	fields := kv.mapper.ToFields(entity)
	columns := kv.table.Columns
	data, err := kv.mapper.Marshal(entity)
	if err != nil {
		return err
	}
	fields["data"] = string(data)
	values := make([]interface{}, 0, len(columns))
	for _, c := range columns {
		values = append(values, fields[c])
	}
	sql := sq.Insert(kv.table.Name).Columns(columns...).Values(values...)
	sql = sql.Suffix(kv.onConflict)
	query, args, err := sql.ToSql()
	if err != nil {
		return err
	}
	_, err = kv.db.Exec(query, args...)
	return err
}

func (kv *EntityRepository[Entity]) AllBy(conditions []sq.Sqlizer) (entities []*Entity, err error) {
	return kv.FilterBy(conditions, make([]storage.Sort, 0), storage.Pagination{0, 0})
}

func (kv *EntityRepository[Entity]) FilterBy(
	conditions []sq.Sqlizer,
	sort []storage.Sort,
	pagination storage.Pagination,
) (entities []*Entity, err error) {
	sql := sq.Select(kv.table.DataField).From(kv.table.Name)
	for _, cond := range conditions {
		sql = sql.Where(cond)
	}
	return kv.Retrieve(sql, pagination, sort)
}

func (kv *EntityRepository[Entity]) Retrieve(sql sq.SelectBuilder, pagination storage.Pagination, sort []storage.Sort) (entities []*Entity, err error) {
	if pagination.Limit > 0 {
		sql = sql.Limit(pagination.Limit)
	}
	if pagination.Offset > 0 {
		sql = sql.Offset(pagination.Offset)
	}
	for _, s := range sort {
		sql = sql.OrderByClause(s.Field + " " + string(s.Order))
	}
	query, args, err := sql.ToSql()
	if err != nil {
		return nil, err
	}
	var data []string
	err = kv.db.Select(&data, query, args...)
	if err != nil {
		return nil, err
	}
	for _, d := range data {
		var entity Entity
		err = kv.mapper.Unmarshal([]byte(d), &entity)
		if err != nil {
			return nil, err
		}
		entities = append(entities, &entity)
	}
	return entities, nil
}

func (kv *EntityRepository[Entity]) GetBy(conditions []sq.Sqlizer) (entity *Entity, err error) {
	entities, err := kv.FilterBy(conditions, make([]storage.Sort, 0), storage.Pagination{0, 1})
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, storage.ErrNotFound
	}
	return entities[0], nil
}

func (kv *EntityRepository[Entity]) GetById(id string) (entity *Entity, err error) {
	return kv.GetBy([]sq.Sqlizer{sq.Eq{"id": id}})
}

func (kv *EntityRepository[Entity]) DeleteBy(conditions []sq.Sqlizer) error {
	sql := sq.Delete(kv.table.Name)
	for _, cond := range conditions {
		sql = sql.Where(cond)
	}
	query, args, err := sql.ToSql()
	if err != nil {
		return err
	}
	_, err = kv.db.Exec(query, args...)
	return err
}

func (kv *EntityRepository[Entity]) DeleteById(id string) error {
	return kv.DeleteBy([]sq.Sqlizer{sq.Eq{"id": id}})
}

func (kv *EntityRepository[Entity]) DeleteAll() error {
	conditions := make([]sq.Sqlizer, 0)
	return kv.DeleteBy(conditions)
}
