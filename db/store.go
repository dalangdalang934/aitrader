package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store 是唯一的 SQLite 连接持有者
type Store struct {
	db *sql.DB
}

// NewStore 打开（或创建）SQLite，启用 WAL + 外键，建所有表
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma: %w", err)
	}

	s := &Store{db: db}
	for _, init := range []func(*Store) error{
		initLearningTable,
		initNewsTable,
		initEquityTable,
		initPositionTable,
	} {
		if err := init(s); err != nil {
			db.Close()
			return nil, fmt.Errorf("init table: %w", err)
		}
	}
	return s, nil
}

// Close 关闭数据库连接
func (s *Store) Close() error {
	return s.db.Close()
}
