package db

import "time"

// EquityPoint 净值历史快照
type EquityPoint struct {
	ID         int64     `json:"id"`
	TraderID   string    `json:"trader_id"`
	Equity     float64   `json:"equity"`
	Balance    float64   `json:"balance"`
	PnL        float64   `json:"pnl"`
	RecordedAt time.Time `json:"recorded_at"`
}

func initEquityTable(s *Store) error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS equity_history (
		id          INTEGER  PRIMARY KEY AUTOINCREMENT,
		trader_id   TEXT     NOT NULL,
		equity      REAL     NOT NULL,
		balance     REAL     NOT NULL,
		pnl         REAL     NOT NULL,
		recorded_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_equity_trader_time
		ON equity_history(trader_id, recorded_at DESC)`)
	return err
}

// InsertEquity 写入一条净值快照
func (s *Store) InsertEquity(traderID string, equity, balance, pnl float64) error {
	_, err := s.db.Exec(
		`INSERT INTO equity_history(trader_id, equity, balance, pnl, recorded_at) VALUES(?,?,?,?,?)`,
		traderID, equity, balance, pnl, time.Now().UTC(),
	)
	return err
}

// QueryEquityHistory 查询净值历史，按时间升序返回
func (s *Store) QueryEquityHistory(traderID string, limit int) ([]EquityPoint, error) {
	if limit <= 0 {
		limit = 10000
	}
	rows, err := s.db.Query(`
		SELECT id, trader_id, equity, balance, pnl, recorded_at
		FROM equity_history
		WHERE trader_id = ?
		ORDER BY recorded_at ASC
		LIMIT ?`, traderID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []EquityPoint
	for rows.Next() {
		var p EquityPoint
		if err := rows.Scan(&p.ID, &p.TraderID, &p.Equity, &p.Balance, &p.PnL, &p.RecordedAt); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
