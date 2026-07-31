package db

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

func Open(databaseURL string) (*sql.DB, error) {
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(30 * time.Minute)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}
