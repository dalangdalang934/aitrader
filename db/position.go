package db

import (
	"fmt"
	"time"
)

// PositionRecord 历史仓位记录
type PositionRecord struct {
	ID          string    `json:"id"`
	TraderID    string    `json:"trader_id"`
	Symbol      string    `json:"symbol"`
	Side        string    `json:"side"`
	EntryPrice  float64   `json:"entry_price"`
	ExitPrice   float64   `json:"exit_price"`
	Quantity    float64   `json:"quantity"`
	Leverage    int       `json:"leverage"`
	PnL         float64   `json:"pnl"`
	PnLPct      float64   `json:"pnl_pct"`
	CloseReason string    `json:"close_reason"`
	OpenedAt    time.Time `json:"opened_at"`
	ClosedAt    time.Time `json:"closed_at"`
}

func initPositionTable(s *Store) error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS position_history (
		id           TEXT     PRIMARY KEY,
		trader_id    TEXT     NOT NULL,
		symbol       TEXT     NOT NULL,
		side         TEXT     NOT NULL,
		entry_price  REAL     NOT NULL,
		exit_price   REAL     NOT NULL DEFAULT 0,
		quantity     REAL     NOT NULL,
		leverage     INTEGER  NOT NULL DEFAULT 1,
		pnl          REAL     NOT NULL DEFAULT 0,
		pnl_pct      REAL     NOT NULL DEFAULT 0,
		close_reason TEXT     NOT NULL DEFAULT '',
		opened_at    DATETIME NOT NULL,
		closed_at    DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_pos_trader_time
		ON position_history(trader_id, closed_at DESC)`)
	return err
}

// InsertPosition 写入历史仓位（REPLACE 支持多次平仓更新加权均价）
func (s *Store) InsertPosition(r PositionRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO position_history
		(id, trader_id, symbol, side, entry_price, exit_price, quantity, leverage,
		 pnl, pnl_pct, close_reason, opened_at, closed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.TraderID, r.Symbol, r.Side,
		r.EntryPrice, r.ExitPrice, r.Quantity, r.Leverage,
		r.PnL, r.PnLPct, r.CloseReason, r.OpenedAt.UTC(), r.ClosedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert position_history: %w", err)
	}
	return nil
}

// QueryPositionHistory 查询历史仓位，按平仓时间倒序
func (s *Store) QueryPositionHistory(traderID string, limit int) ([]PositionRecord, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(`
		SELECT id, trader_id, symbol, side, entry_price, exit_price, quantity, leverage,
		       pnl, pnl_pct, close_reason, opened_at, closed_at
		FROM position_history
		WHERE trader_id = ?
		ORDER BY closed_at DESC
		LIMIT ?`, traderID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []PositionRecord
	for rows.Next() {
		var r PositionRecord
		if err := rows.Scan(
			&r.ID, &r.TraderID, &r.Symbol, &r.Side,
			&r.EntryPrice, &r.ExitPrice, &r.Quantity, &r.Leverage,
			&r.PnL, &r.PnLPct, &r.CloseReason, &r.OpenedAt, &r.ClosedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
