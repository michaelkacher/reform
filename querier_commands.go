package reform

import (
	"fmt"
	"strings"
)

// Insert inserts a struct into SQL database table.
// If str implements BeforeInserter, it calls BeforeInsert() before doing so.
func (q *Querier) Insert(str Struct) error {
	if bi, ok := str.(BeforeInserter); ok {
		err := bi.BeforeInsert()
		if err != nil {
			return err
		}
	}

	view := str.View()
	values := str.Values()
	columns := view.Columns()
	record, _ := str.(Record)
	var pk uint

	if record != nil {
		pk = view.(Table).PKColumnIndex()

		// cut primary key
		if !record.HasPK() {
			values = append(values[:pk], values[pk+1:]...)
			columns = append(columns[:pk], columns[pk+1:]...)
		}
	}

	for i, c := range columns {
		columns[i] = q.QuoteIdentifier(c)
	}
	placeholders := q.Placeholders(1, len(columns))

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		q.QuoteIdentifier(view.Name()),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	switch q.Dialect.LastInsertIdMethod() {
	case LastInsertId:
		res, err := q.Exec(query, values...)
		if err != nil {
			return err
		}
		if record != nil {
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			record.SetPK(id)
		}
		return nil

	case Returning:
		var err error
		if record != nil {
			query += fmt.Sprintf(" RETURNING %s", q.QuoteIdentifier(view.Columns()[pk]))
			err = q.QueryRow(query, values...).Scan(record.PKPointer())
		} else {
			_, err = q.Exec(query, values...)
		}
		return err

	default:
		panic("reform: Unhandled LastInsertIdMethod. Please report this bug.")
	}
}

func (q *Querier) update(record Record, columns []string, values []interface{}) error {
	for i, c := range columns {
		columns[i] = q.QuoteIdentifier(c)
	}
	placeholders := q.Placeholders(1, len(columns))

	p := make([]string, len(columns))
	for i, c := range columns {
		p[i] = c + " = " + placeholders[i]
	}
	table := record.Table()
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s",
		q.QuoteIdentifier(table.Name()),
		strings.Join(p, ", "),
		q.QuoteIdentifier(table.Columns()[table.PKColumnIndex()]),
		q.Placeholder(len(columns)+1),
	)

	args := append(values, record.PKValue())
	res, err := q.Exec(query, args...)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return ErrNoRows
	}
	if ra > 1 {
		panic(fmt.Errorf("reform: %d rows by UPDATE by primary key. Please report this bug.", ra))
	}
	return nil
}

func (q *Querier) beforeUpdate(record Record) error {
	if !record.HasPK() {
		return ErrNoPK
	}

	if bu, ok := record.(BeforeUpdater); ok {
		err := bu.BeforeUpdate()
		if err != nil {
			return err
		}
	}

	return nil
}

// Update updates all columns of row specified by primary key in SQL database table with given record.
// If record implements BeforeUpdater, it calls BeforeUpdate() before doing so.
//
// Method returns ErrNoRows if no rows were updated.
// Method returns ErrNoPK if primary key is not set.
func (q *Querier) Update(record Record) error {
	err := q.beforeUpdate(record)
	if err != nil {
		return err
	}

	table := record.Table()
	values := record.Values()
	columns := table.Columns()

	// cut primary key
	pk := table.PKColumnIndex()
	values = append(values[:pk], values[pk+1:]...)
	columns = append(columns[:pk], columns[pk+1:]...)

	return q.update(record, columns, values)
}

// UpdateColumns updates specified columns of row specified by primary key in SQL database table with given record.
// If record implements BeforeUpdater, it calls BeforeUpdate() before doing so.
//
// Method returns ErrNoRows if no rows were updated.
// Method returns ErrNoPK if primary key is not set.
func (q *Querier) UpdateColumns(record Record, columns ...string) error {
	err := q.beforeUpdate(record)
	if err != nil {
		return err
	}

	columnsSet := make(map[string]struct{}, len(columns))
	for _, c := range columns {
		columnsSet[c] = struct{}{}
	}

	table := record.Table()
	allColumns := table.Columns()
	allValues := record.Values()
	columns = make([]string, 0, len(columnsSet))
	values := make([]interface{}, 0, len(columns))
	for i, c := range allColumns {
		if _, ok := columnsSet[c]; ok {
			delete(columnsSet, c)
			columns = append(columns, c)
			values = append(values, allValues[i])
		}
	}

	if len(columnsSet) > 0 {
		columns = make([]string, 0, len(columnsSet))
		for c := range columnsSet {
			columns = append(columns, c)
		}
		// TODO make exported type for that error
		return fmt.Errorf("reform: unexpected columns: %v", columns)
	}

	if len(values) == 0 {
		// TODO make exported type for that error
		return fmt.Errorf("reform: nothing to update")
	}

	return q.update(record, columns, values)
}

// Save saves record in SQL database table.
// If primary key is set, it first calls Update and checks if row was updated.
// If primary key is absent or no row was updated, it calls Insert.
func (q *Querier) Save(record Record) error {
	if record.HasPK() {
		err := q.Update(record)
		if err != ErrNoRows {
			return err
		}
	}

	return q.Insert(record)
}

// Delete deletes record from SQL database table by primary key.
//
// Method returns ErrNoRows if no rows were deleted.
// Method returns ErrNoPK if primary key is not set.
func (q *Querier) Delete(record Record) error {
	if !record.HasPK() {
		return ErrNoPK
	}

	table := record.Table()
	pk := table.PKColumnIndex()
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = %s",
		q.QuoteIdentifier(table.Name()),
		q.QuoteIdentifier(table.Columns()[pk]),
		q.Placeholder(1),
	)

	res, err := q.Exec(query, record.PKValue())
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return ErrNoRows
	}
	if ra > 1 {
		panic(fmt.Errorf("reform: %d rows by DELETE by primary key. Please report this bug.", ra))
	}
	return nil
}

// DeleteFrom deletes rows from view with tail and args and returns a number of deleted rows.
//
// Method never returns ErrNoRows.
func (q *Querier) DeleteFrom(view View, tail string, args ...interface{}) (uint, error) {
	query := fmt.Sprintf("DELETE FROM %s %s",
		q.QuoteIdentifier(view.Name()),
		tail,
	)

	res, err := q.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return uint(ra), nil
}
