package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	*sqlRepository
}

func OpenSQLite(ctx context.Context, dsn string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // modernc.org/sqlite: avoid "database is locked" under concurrent writers
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: ping sqlite: %w", err)
	}
	return &SQLiteRepository{sqlRepository: &sqlRepository{db: db, dialect: "sqlite"}}, nil
}

func (r *SQLiteRepository) DB() *sql.DB  { return r.db }
func (r *SQLiteRepository) Close() error { return r.db.Close() }
