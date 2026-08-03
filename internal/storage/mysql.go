package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLRepository struct {
	*sqlRepository
}

func OpenMySQL(ctx context.Context, dsn string) (*MySQLRepository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open mysql: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: ping mysql: %w", err)
	}
	return &MySQLRepository{sqlRepository: &sqlRepository{db: db, dialect: "mysql"}}, nil
}

func (r *MySQLRepository) DB() *sql.DB  { return r.db }
func (r *MySQLRepository) Close() error { return r.db.Close() }
