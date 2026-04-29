package trader

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aitrade/db"
	"github.com/google/uuid"
)

// positionWriter 私有接口：仅需 InsertPosition
type positionWriter interface {
	InsertPosition(r db.PositionRecord) error
}

// PositionStatus 仓位状态
type PositionStatus string

const (
	PositionStatusOpen          PositionStatus = "open"
	PositionStatusPartialClosed PositionStatus = "partial_closed"
	PositionStatusClosed        PositionStatus = "closed"
)

// Position 代表一个完整的交易仓位
type Position struct {
	ID           string    `json:"id"`            // 唯一标识 (UUID)
	Symbol       string    `json:"symbol"`        // 交易对
	Side         string    `json:"side"`          // long/short
	OpenTime     time.Time `json:"open_time"`     // 开仓时间
	OpenPrice    float64   `json:"open_price"`    // 开仓价格
	OpenQuantity float64   `json:"open_quantity"` // 开仓数量
	OpenOrderID  string    `json:"open_order_id"` // 开仓订单ID
	Leverage     int       `json:"leverage"`      // 杠杆倍数
	MarkPrice    float64   `json:"mark_price"`    // 最新标记价格

	// 平仓记录
	Closes []PositionClose `json:"closes"` // 所有平仓记录

	// 状态
	Status       PositionStatus `json:"status"`        // 仓位状态
	RemainingQty float64        `json:"remaining_qty"` // 剩余数量

	// 统计
	RealizedPnL      float64 `json:"realized_pnl"`       // 已实现盈亏
	TotalCommission  float64 `json:"total_commission"`   // 总手续费
	UnrealizedPnL    float64 `json:"unrealized_pnl"`     // 未实现盈亏
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"` // 未实现盈亏百分比

	// 元数据
	CreatedAt time.Time `json:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at"` // 更新时间
}

// PositionClose 代表一次平仓操作
type PositionClose struct {
	CloseTime    time.Time `json:"close_time"`     // 平仓时间
	ClosePrice   float64   `json:"close_price"`    // 平仓价格
	CloseQty     float64   `json:"close_qty"`      // 平仓数量
	CloseOrderID string    `json:"close_order_id"` // 平仓订单ID
	PnL          float64   `json:"pnl"`            // 本次平仓盈亏
	Commission   float64   `json:"commission"`     // 本次手续费
	Reason       string    `json:"reason"`         // 平仓原因
}

// PositionTracker 仓位追踪器
type PositionTracker struct {
	mu              sync.RWMutex
	activePositions map[string]*Position // positionID -> Position
	traderID        string
	dataDir         string
	writer          positionWriter // 可选，平仓时写 position_history
}

// NewPositionTracker 创建新的仓位追踪器
func NewPositionTracker(traderID string, writer positionWriter) *PositionTracker {
	dataDir := filepath.Join("data", "positions")
	os.MkdirAll(dataDir, 0755)

	pt := &PositionTracker{
		activePositions: make(map[string]*Position),
		traderID:        traderID,
		dataDir:         dataDir,
		writer:          writer,
	}

	// 加载活跃仓位
	if err := pt.loadActivePositions(); err != nil {
		log.Printf("⚠️ [%s] 加载活跃仓位失败: %v", traderID, err)
	}
	return pt
}

// CreatePosition 创建新仓位
func (pt *PositionTracker) CreatePosition(symbol, side string, openPrice, quantity float64, leverage int, orderID string) *Position {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if existing := pt.findActivePositionLocked(symbol, side); existing != nil {
		return clonePosition(existing)
	}

	position := &Position{
		ID:           uuid.New().String(),
		Symbol:       symbol,
		Side:         side,
		OpenTime:     time.Now(),
		OpenPrice:    openPrice,
		OpenQuantity: quantity,
		OpenOrderID:  orderID,
		Leverage:     leverage,
		MarkPrice:    openPrice,
		Closes:       []PositionClose{},
		Status:       PositionStatusOpen,
		RemainingQty: quantity,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	pt.activePositions[position.ID] = position
	if err := pt.saveActivePositions(); err != nil {
		log.Printf("⚠️ [%s] 保存活跃仓位失败: %v", pt.traderID, err)
	} else {
		log.Printf("✅ [%s] 创建仓位成功: %s %s %s @ %.4f, ID: %s (活跃仓位数: %d)",
			pt.traderID, side, fmt.Sprintf("%.4f", quantity), symbol, openPrice, position.ID, len(pt.activePositions))
	}

	return position
}

// ClosePosition 平仓（支持部分平仓）
func (pt *PositionTracker) ClosePosition(positionID string, closePrice, quantity float64, pnl, commission float64, orderID, reason string) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	position, exists := pt.activePositions[positionID]
	if !exists {
		return fmt.Errorf("仓位不存在: %s", positionID)
	}

	if quantity > position.RemainingQty {
		return fmt.Errorf("平仓数量超过剩余数量: %.4f > %.4f", quantity, position.RemainingQty)
	}

	before := clonePosition(position)

	// 记录平仓
	closeRecord := PositionClose{
		CloseTime:    time.Now(),
		ClosePrice:   closePrice,
		CloseQty:     quantity,
		CloseOrderID: orderID,
		PnL:          pnl,
		Commission:   commission,
		Reason:       reason,
	}

	position.Closes = append(position.Closes, closeRecord)
	position.RemainingQty -= quantity
	position.RealizedPnL += pnl
	position.TotalCommission += commission
	position.MarkPrice = closePrice
	position.UpdatedAt = time.Now()

	if position.RemainingQty > quantityEpsilon {
		if position.Side == "long" {
			position.UnrealizedPnL = (closePrice - position.OpenPrice) * position.RemainingQty
		} else {
			position.UnrealizedPnL = (position.OpenPrice - closePrice) * position.RemainingQty
		}
		position.UnrealizedPnLPct = calcUnrealizedPct(position.OpenPrice, position.UnrealizedPnL, position.RemainingQty)
	} else {
		position.UnrealizedPnL = 0
		position.UnrealizedPnLPct = 0
	}

	// 更新状态
	closed := position.RemainingQty <= quantityEpsilon
	if closed {
		position.Status = PositionStatusClosed
		position.RemainingQty = 0
		position.UnrealizedPnL = 0
		position.UnrealizedPnLPct = 0
	} else {
		position.Status = PositionStatusPartialClosed
	}

	if closed {
		if err := pt.moveToHistory(clonePosition(position)); err != nil {
			restorePosition(position, before)
			return fmt.Errorf("移动仓位到历史失败: %w", err)
		}
		delete(pt.activePositions, positionID)
	}

	if err := pt.saveActivePositions(); err != nil {
		if closed {
			pt.activePositions[positionID] = before
		} else {
			restorePosition(position, before)
		}
		if rollbackErr := pt.saveActivePositions(); rollbackErr != nil {
			log.Printf("⚠️ [%s] 回滚活跃仓位失败: %v", pt.traderID, rollbackErr)
		}
		return fmt.Errorf("保存活跃仓位失败: %w", err)
	}

	return nil
}

// GetPosition 获取仓位信息
func (pt *PositionTracker) GetPosition(positionID string) (*Position, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	if position, exists := pt.activePositions[positionID]; exists {
		return clonePosition(position), nil
	}

	// 尝试从历史记录中查找
	return pt.loadPositionFromHistory(positionID)
}

// GetActivePositions 获取所有活跃仓位
func (pt *PositionTracker) GetActivePositions() []*Position {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	positions := make([]*Position, 0, len(pt.activePositions))
	for _, pos := range pt.activePositions {
		if pos.Status == PositionStatusClosed {
			continue
		}
		positions = append(positions, clonePosition(pos))
	}
	return positions
}

// GetActivePositionBySymbol 根据币种获取活跃仓位
func (pt *PositionTracker) GetActivePositionBySymbol(symbol, side string) *Position {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return clonePosition(pt.findActivePositionLocked(symbol, side))
}

// 保存活跃仓位到文件
func (pt *PositionTracker) saveActivePositions() error {
	fileName := filepath.Join(pt.dataDir, fmt.Sprintf("%s_active.json", pt.traderID))

	// 将map转换为数组
	positions := make([]*Position, 0, len(pt.activePositions))
	for _, pos := range pt.activePositions {
		positions = append(positions, clonePosition(pos))
	}

	data, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化活跃仓位失败: %w", err)
	}

	return os.WriteFile(fileName, data, 0644)
}

// 加载活跃仓位
func (pt *PositionTracker) loadActivePositions() error {
	fileName := filepath.Join(pt.dataDir, fmt.Sprintf("%s_active.json", pt.traderID))

	data, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在是正常的
		}
		return fmt.Errorf("读取活跃仓位失败: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	// 先解析为数组，然后转换为map
	var positions []*Position
	if err := json.Unmarshal(data, &positions); err != nil {
		// 兼容旧格式（map格式）
		var oldMap map[string]*Position
		if err2 := json.Unmarshal(data, &oldMap); err2 == nil {
			pt.activePositions = make(map[string]*Position)
			for id, pos := range oldMap {
				if pos == nil {
					continue
				}
				pt.activePositions[id] = clonePosition(pos)
			}
			return nil
		}
		return fmt.Errorf("解析活跃仓位失败: %w", err)
	}

	// 转换为map
	pt.activePositions = make(map[string]*Position)
	for _, pos := range positions {
		pt.activePositions[pos.ID] = clonePosition(pos)
	}

	return nil
}

// 将仓位移到历史记录
func (pt *PositionTracker) moveToHistory(position *Position) error {
	// 同时写入 SQLite
	if pt.writer != nil && len(position.Closes) > 0 {
		// 计算加权均价 exit_price
		var totalQty, totalValue float64
		for _, c := range position.Closes {
			totalQty += c.CloseQty
			totalValue += c.ClosePrice * c.CloseQty
		}
		exitPrice := 0.0
		if totalQty > 0 {
			exitPrice = totalValue / totalQty
		}
		lastClose := position.Closes[len(position.Closes)-1]

		costBasis := position.OpenPrice * position.OpenQuantity
		pnlPct := 0.0
		if costBasis > 0 {
			pnlPct = position.RealizedPnL / costBasis * 100
		}
		r := db.PositionRecord{
			ID:          position.ID,
			TraderID:    pt.traderID,
			Symbol:      position.Symbol,
			Side:        position.Side,
			EntryPrice:  position.OpenPrice,
			ExitPrice:   exitPrice,
			Quantity:    position.OpenQuantity,
			Leverage:    position.Leverage,
			PnL:         position.RealizedPnL,
			PnLPct:      pnlPct,
			CloseReason: lastClose.Reason,
			OpenedAt:    position.OpenTime,
			ClosedAt:    lastClose.CloseTime,
		}
		if err := pt.writer.InsertPosition(r); err != nil {
			log.Printf("⚠️ 写入历史仓位失败: %v", err)
		}
	}

	historyFile := filepath.Join(pt.dataDir, fmt.Sprintf("%s_history.json", pt.traderID))

	// 读取现有历史
	var history []*Position
	data, err := os.ReadFile(historyFile)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &history); err != nil {
			return fmt.Errorf("解析历史仓位失败: %w", err)
		}
	}

	// 添加新记录
	cloned := clonePosition(position)
	history = append([]*Position{cloned}, history...) // 新记录在前

	// 限制历史记录数量
	if len(history) > 1000 {
		history = history[:1000]
	}

	// 保存
	data, err = json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化历史仓位失败: %w", err)
	}

	return os.WriteFile(historyFile, data, 0644)
}

// 从历史记录加载仓位
func (pt *PositionTracker) loadPositionFromHistory(positionID string) (*Position, error) {
	historyFile := filepath.Join(pt.dataDir, fmt.Sprintf("%s_history.json", pt.traderID))

	data, err := os.ReadFile(historyFile)
	if err != nil {
		return nil, fmt.Errorf("读取历史仓位失败: %w", err)
	}

	var history []*Position
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("解析历史仓位失败: %w", err)
	}

	for _, pos := range history {
		if pos.ID == positionID {
			return clonePosition(pos), nil
		}
	}

	return nil, fmt.Errorf("仓位不存在: %s", positionID)
}

// GetAllHistory 获取所有历史仓位（包括活跃和已关闭）
func (pt *PositionTracker) GetAllHistory() ([]*Position, error) {
	// 合并活跃和已关闭的仓位
	allPositions := make([]*Position, 0)

	// 添加活跃仓位
	activePositions := pt.GetActivePositions()
	for _, pos := range activePositions {
		allPositions = append(allPositions, clonePosition(pos))
	}

	// 读取已关闭仓位文件
	closedFile := filepath.Join(pt.dataDir, fmt.Sprintf("%s_history.json", pt.traderID))
	data, err := os.ReadFile(closedFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取已关闭仓位失败: %w", err)
	}

	if len(data) > 0 {
		var closedPositions []*Position
		if err := json.Unmarshal(data, &closedPositions); err != nil {
			return nil, fmt.Errorf("解析已关闭仓位失败: %w", err)
		}
		for _, pos := range closedPositions {
			allPositions = append(allPositions, clonePosition(pos))
		}
	}

	return allPositions, nil
}

const quantityEpsilon = 1e-8

// SyncWithExchange 使用交易所当前持仓状态刷新活跃仓位
func (pt *PositionTracker) SyncWithExchange(positions []map[string]interface{}) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	seen := make(map[string]bool)
	now := time.Now()

	for _, raw := range positions {
		symbol, _ := raw["symbol"].(string)
		side, _ := raw["side"].(string)
		side = strings.ToLower(side)
		if symbol == "" || side == "" {
			continue
		}

		quantity := math.Abs(toFloat(raw["positionAmt"]))
		if quantity <= quantityEpsilon {
			continue
		}

		entryPrice := toFloat(raw["entryPrice"])
		markPrice := toFloat(raw["markPrice"])
		unrealizedPnL := toFloat(raw["unRealizedProfit"])
		leverage := int(toFloat(raw["leverage"]))
		if leverage <= 0 {
			leverage = 1
		}

		pnlPct := calcUnrealizedPct(entryPrice, unrealizedPnL, quantity)

		key := symbol + "_" + side
		seen[key] = true

		position := pt.findActivePositionLocked(symbol, side)
		if position == nil {
			position = &Position{
				ID:               uuid.New().String(),
				Symbol:           symbol,
				Side:             side,
				OpenTime:         now,
				OpenPrice:        entryPrice,
				OpenQuantity:     quantity,
				OpenOrderID:      "",
				Leverage:         leverage,
				MarkPrice:        markPrice,
				Status:           PositionStatusOpen,
				RemainingQty:     quantity,
				UnrealizedPnL:    unrealizedPnL,
				UnrealizedPnLPct: pnlPct,
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			pt.activePositions[position.ID] = position
			continue
		}

		prevQty := position.RemainingQty
		prevOpenPrice := position.OpenPrice
		prevMarkPrice := position.MarkPrice

		// 先记录部分/全部平仓
		if prevQty > quantity+quantityEpsilon {
			closedQty := prevQty - quantity
			closePrice := markPrice
			if closePrice <= 0 {
				closePrice = prevMarkPrice
				if closePrice <= 0 {
					closePrice = prevOpenPrice
				}
			}
			realizedDelta := computeRealizedPnL(side, prevOpenPrice, closePrice, closedQty)
			position.RealizedPnL += realizedDelta
			position.Closes = append(position.Closes, PositionClose{
				CloseTime:  now,
				ClosePrice: closePrice,
				CloseQty:   closedQty,
				PnL:        realizedDelta,
				Reason:     "sync_partial_close",
			})
		}

		// 如果加仓，使用交易所返回数据更新平均价格/头寸
		if quantity > prevQty+quantityEpsilon && entryPrice > 0 {
			position.OpenPrice = entryPrice
		}

		position.OpenQuantity = quantity
		position.RemainingQty = quantity
		position.Leverage = leverage
		position.MarkPrice = markPrice
		position.UnrealizedPnL = unrealizedPnL
		position.UnrealizedPnLPct = calcUnrealizedPct(position.OpenPrice, position.UnrealizedPnL, position.RemainingQty)
		if quantity > quantityEpsilon {
			if quantity < prevQty-quantityEpsilon {
				position.Status = PositionStatusPartialClosed
			} else {
				position.Status = PositionStatusOpen
			}
		} else {
			position.Status = PositionStatusClosed
		}
		position.UpdatedAt = now
	}

	// 处理已关闭的仓位
	for id, pos := range pt.activePositions {
		key := pos.Symbol + "_" + pos.Side
		if seen[key] {
			continue
		}
		// 已不在交易所中，移入历史
		if pos.RemainingQty > quantityEpsilon {
			closePrice := pos.MarkPrice
			if closePrice <= 0 {
				closePrice = pos.OpenPrice
			}
			realizedDelta := computeRealizedPnL(pos.Side, pos.OpenPrice, closePrice, pos.RemainingQty)
			pos.RealizedPnL += realizedDelta
			pos.Closes = append(pos.Closes, PositionClose{
				CloseTime:  now,
				ClosePrice: closePrice,
				CloseQty:   pos.RemainingQty,
				PnL:        realizedDelta,
				Reason:     "sync_close",
			})
		}
		pos.Status = PositionStatusClosed
		pos.RemainingQty = 0
		pos.UnrealizedPnL = 0
		pos.UnrealizedPnLPct = 0
		pos.UpdatedAt = now
		if err := pt.moveToHistory(clonePosition(pos)); err != nil {
			log.Printf("⚠️ [%s] 移动历史仓位失败: %v", pt.traderID, err)
		}
		delete(pt.activePositions, id)
	}

	if err := pt.saveActivePositions(); err != nil {
		log.Printf("⚠️ [%s] 同步活跃仓位失败: %v", pt.traderID, err)
	}
}

// findActivePositionLocked assumes caller holds at least read lock.
func (pt *PositionTracker) findActivePositionLocked(symbol, side string) *Position {
	for _, pos := range pt.activePositions {
		if pos.Symbol == symbol && pos.Side == side && pos.Status != PositionStatusClosed {
			return pos
		}
	}
	return nil
}

// clonePosition 生成仓位的深拷贝
func clonePosition(pos *Position) *Position {
	if pos == nil {
		return nil
	}
	copyPos := *pos
	if len(pos.Closes) > 0 {
		copyPos.Closes = make([]PositionClose, len(pos.Closes))
		copy(copyPos.Closes, pos.Closes)
	}
	return &copyPos
}

// restorePosition 用备份内容覆盖原仓位
func restorePosition(dst *Position, src *Position) {
	if dst == nil || src == nil {
		return
	}
	*dst = *src
	if len(src.Closes) > 0 {
		dst.Closes = make([]PositionClose, len(src.Closes))
		copy(dst.Closes, src.Closes)
	} else {
		dst.Closes = nil
	}
}

func toFloat(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func computeRealizedPnL(side string, openPrice, closePrice, quantity float64) float64 {
	if quantity <= 0 {
		return 0
	}
	switch side {
	case "long":
		return (closePrice - openPrice) * quantity
	case "short":
		return (openPrice - closePrice) * quantity
	default:
		return 0
	}
}

func calcUnrealizedPct(entryPrice, unrealizedPnL, quantity float64) float64 {
	if quantity <= quantityEpsilon {
		return 0
	}
	notional := entryPrice * quantity
	if notional == 0 {
		return 0
	}
	return (unrealizedPnL / notional) * 100
}
