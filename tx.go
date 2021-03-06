package reform

import (
	"database/sql"
	"time"
)

// TX represents a SQL database transaction.
type TX struct {
	*Querier
	tx *sql.Tx
}

// NewTX creates new TX object for given SQL database transaction.
func NewTX(tx *sql.Tx, dialect Dialect, logger Logger) *TX {
	return &TX{
		Querier: newQuerier(tx, dialect, logger),
		tx:      tx,
	}
}

// Commit commits the transaction.
func (tx *TX) Commit() error {
	start := time.Now()
	tx.logBefore("COMMIT", nil)
	err := tx.tx.Commit()
	tx.logAfter("COMMIT", nil, time.Now().Sub(start), err)
	return err
}

// Rollback aborts the transaction.
func (tx *TX) Rollback() error {
	start := time.Now()
	tx.logBefore("ROLLBACK", nil)
	err := tx.tx.Rollback()
	tx.logAfter("ROLLBACK", nil, time.Now().Sub(start), err)
	return err
}

// check interface
var _ DBTX = new(TX)
