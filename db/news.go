package db

import (
	"database/sql"
	"fmt"
	"time"
)

func initNewsTable(s *Store) error {
	// 保留旧列结构以兼容已有数据（digest_data 列无害）
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS news_cache (
		id          INTEGER  PRIMARY KEY,
		raw_data    TEXT     NOT NULL,
		digest_data TEXT     NOT NULL DEFAULT '',
		updated_at  DATETIME NOT NULL
	)`)
	return err
}

// LoadNewsCache 读取新闻缓存原始数据
func (s *Store) LoadNewsCache() ([]byte, error) {
	var raw string
	err := s.db.QueryRow(`SELECT raw_data FROM news_cache WHERE id=1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load news cache: %w", err)
	}
	return []byte(raw), nil
}

// SaveNewsCache 写入新闻缓存（Upsert，始终保持单行）
func (s *Store) SaveNewsCache(rawData []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO news_cache(id, raw_data, digest_data, updated_at) VALUES(1,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET raw_data=excluded.raw_data, updated_at=excluded.updated_at`,
		string(rawData), "", time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save news cache: %w", err)
	}
	return nil
}
