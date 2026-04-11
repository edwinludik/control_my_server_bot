package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

const dbName = ".user_ids.db"

type UserStore struct {
	db *sql.DB
}

func NewUserStore() (store *UserStore, err error) {
	return NewUserStoreWithDB(dbName)
}

func NewUserStoreWithDB(path string) (store *UserStore, err error) {
	// Use DSN with WAL mode and busy timeout for better SQLite performance and reliability
	var dsn string
	if path == ":memory:" {
		dsn = path
	} else {
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Defer database closure on error
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Ensure the database file has restricted permissions if it's a file
	if path != ":memory:" {
		if _, err := os.Stat(path); err == nil {
			if err := os.Chmod(path, 0600); err != nil {
				return nil, fmt.Errorf("chmod database: %w", err)
			}
		}
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY
	)`)
	if err != nil {
		return nil, fmt.Errorf("create users table: %w", err)
	}

	return &UserStore{db: db}, nil
}

func (s *UserStore) AddUser(id int64) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO users (id) VALUES (?)", id)
	return err
}

func (s *UserStore) DeleteUser(id int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *UserStore) UserExists(id int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

func (s *UserStore) ListUsers(ownerID int64) ([]string, error) {
	rows, err := s.db.Query("SELECT id FROM users")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var result []string
	result = append(result, fmt.Sprintf("User: %d (Owner, cannot be deleted)", ownerID))

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("User: %d", id))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserStore) Close() error {
	return s.db.Close()
}
