package db

import (
	"database/sql"
	"fmt"
	"time"
)

func initLearningTable(s *Store) error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS learning_states (
		trader_id  TEXT     PRIMARY KEY,
		data       TEXT     NOT NULL,
		updated_at DATETIME NOT NULL
	)`)
	return err
}

// LoadLearning 读取学习状态 JSON，不存在返回 nil
func (s *Store) LoadLearning(traderID string) ([]byte, error) {
	var data string
	err := s.db.QueryRow(`SELECT data FROM learning_states WHERE trader_id = ?`, traderID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query learning state: %w", err)
	}
	return []byte(data), nil
}

// SaveLearning 写入已序列化的学习状态（Upsert）
func (s *Store) SaveLearning(traderID string, data []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO learning_states(trader_id, data, updated_at) VALUES(?,?,?)
		 ON CONFLICT(trader_id) DO UPDATE SET data=excluded.data, updated_at=excluded.updated_at`,
		traderID, string(data), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert learning state: %w", err)
	}
	return nil
}
