package db

// Writer 写入接口（供 trader/learning/news 使用）
type Writer interface {
	InsertEquity(traderID string, equity, balance, pnl float64) error
	InsertPosition(r PositionRecord) error
	SaveLearning(traderID string, data []byte) error
	SaveNewsCache(rawData []byte) error
}

// AIReader 内部读取接口（供 learning/news 使用）
type AIReader interface {
	LoadLearning(traderID string) ([]byte, error)
	LoadNewsCache() ([]byte, error)
}

// APIReader 查询接口（供 api 服务器使用）
type APIReader interface {
	QueryEquityHistory(traderID string, limit int) ([]EquityPoint, error)
	QueryPositionHistory(traderID string, limit int) ([]PositionRecord, error)
}
