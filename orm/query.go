package orm

import (
	"fmt"
	"reflect"

	"gopkg.in/pg.v4/types"
)

type Query struct {
	db    dber
	model TableModel
	err   error

	tables    []byte
	fields    []string
	columns   []byte
	returning []byte
	where     []byte
	join      []byte
	order     []byte
	limit     int
	offset    int
}

func NewQuery(db dber, v interface{}) *Query {
	model, err := NewTableModel(v)
	q := Query{
		db:    db,
		model: model,
		err:   err,
	}
	if err == nil {
		q.tables = types.AppendField(q.tables, q.model.Table().Name, 1)
		q.tables = append(q.tables, " AS "...)
		q.tables = types.AppendField(q.tables, q.model.Table().ModelName, 1)
	}
	return &q
}

func (q *Query) setErr(err error) {
	if q.err == nil {
		q.err = err
	}
}

func (q *Query) Table(names ...string) *Query {
	for _, name := range names {
		q.tables = types.AppendField(q.tables, name, 1)
	}
	return q
}

func (q *Query) Column(columns ...interface{}) *Query {
loop:
	for _, column := range columns {
		switch column := column.(type) {
		case string:
			if _, err := q.model.Join(column); err == nil {
				continue loop
			}
			q.fields = append(q.fields, column)

			q.columns = appendSep(q.columns, ", ")
			q.columns = types.AppendField(q.columns, column, 1)
		case types.ValueAppender:
			var err error
			q.columns = appendSep(q.columns, ", ")
			q.columns, err = column.AppendValue(q.columns, 1)
			if err != nil {
				q.setErr(err)
			}
		default:
			q.setErr(fmt.Errorf("unsupported column type: %T", column))
		}
	}
	return q
}

func (q *Query) Returning(columns ...interface{}) *Query {
	for _, column := range columns {
		q.returning = appendSep(q.returning, ", ")

		switch column := column.(type) {
		case string:
			q.returning = types.AppendField(q.returning, column, 1)
		case types.ValueAppender:
			var err error
			q.returning, err = column.AppendValue(q.returning, 1)
			if err != nil {
				q.setErr(err)
			}
		default:
			q.setErr(fmt.Errorf("unsupported column type: %T", column))
		}
	}
	return q
}

func (q *Query) Where(where string, params ...interface{}) *Query {
	if false {
		for i, param := range params {
			if f, ok := param.(types.F); ok {
				column, err := q.model.Join(string(f) + "._")
				if err == nil {
					params[i] = types.F(column)
				}
			}
		}
	}

	q.where = appendSep(q.where, " AND ")
	q.where = append(q.where, '(')
	q.where = Formatter{}.Append(q.where, where, params...)
	q.where = append(q.where, ')')

	return q
}

func (q *Query) Join(join string, params ...interface{}) *Query {
	q.join = appendSep(q.join, " ")
	q.join = Formatter{}.Append(q.join, join, params...)
	return q
}

func (q *Query) Order(order string, params ...interface{}) *Query {
	q.order = appendSep(q.join, ", ")
	q.order = Formatter{}.Append(q.order, order, params...)
	return q
}

func (q *Query) Limit(n int) *Query {
	q.limit = n
	return q
}

func (q *Query) Offset(n int) *Query {
	q.offset = n
	return q
}

func (q *Query) Count() (int, error) {
	if q.err != nil {
		return 0, q.err
	}

	q.columns = types.Q("COUNT(*)")
	q.order = nil
	q.limit = 0
	q.offset = 0

	joins := q.model.GetJoins()
	for i := range joins {
		j := &joins[i]
		if j.Rel.One {
			j.JoinOne(q)
		}
	}

	sel := &selectQuery{
		Query: q,
	}

	var count int
	_, err := q.db.Query(Scan(&count), sel, q.model)
	return count, err
}

func (q *Query) First() error {
	b := columns("", q.model.Table().PKs)
	return q.Order(string(b)).Limit(1).Select()
}

func (q *Query) Last() error {
	b := columns("", q.model.Table().PKs)
	b = append(b, " DESC"...)
	return q.Order(string(b)).Limit(1).Select()
}

func (q *Query) Select() error {
	return q.selectModel(q.model)
}

func (q *Query) selectModel(model TableModel) error {
	joins := model.GetJoins()
	for i := range joins {
		j := &joins[i]
		if j.Rel.One {
			j.JoinOne(q)
		}
	}

	sel := &selectQuery{
		Query: q,
	}
	var err error
	if model.Kind() == reflect.Slice {
		_, err = q.db.Query(model, sel, model)
	} else {
		_, err = q.db.QueryOne(model, sel, model)
	}
	if err != nil {
		return err
	}

	return selectJoins(q.db, joins)
}

func selectJoins(db dber, joins []Join) error {
	var err error
	for i := range joins {
		j := &joins[i]
		if j.Rel.One {
			err = selectJoins(db, j.JoinModel.GetJoins())
		} else {
			err = j.Select(db)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (q *Query) Update() error {
	if q.err != nil {
		return q.err
	}
	upd := &updateModel{
		Query: q,
	}
	_, err := q.db.Query(q.model, upd, upd.model)
	return err
}

func (q *Query) UpdateValues(data map[string]interface{}) error {
	if q.err != nil {
		return q.err
	}
	upd := &updateQuery{
		Query: q,
		data:  data,
	}
	_, err := q.db.Query(upd.model, upd, upd.model)
	return err
}

func (q *Query) Delete() error {
	if q.err != nil {
		return q.err
	}
	del := deleteQuery{
		Query: q,
	}
	_, err := q.db.Exec(del, del.model)
	return err
}