package trader

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// FuturesTrader 币安合约交易器
type FuturesTrader struct {
	client *futures.Client

	// 余额缓存
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// 持仓缓存
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex

	// 缓存有效期（15秒）
	cacheDuration     time.Duration
	defaultMarginType futures.MarginType
}

// NewFuturesTrader 创建合约交易器
func NewFuturesTrader(apiKey, secretKey string) *FuturesTrader {
	client := futures.NewClient(apiKey, secretKey)
	// 自动同步 Binance 服务器时间，修正本地时钟偏差
	if _, err := client.NewSetServerTimeService().Do(context.Background()); err != nil {
		log.Printf("⚠️ 同步 Binance 服务器时间失败: %v", err)
	}
	return &FuturesTrader{
		client:            client,
		cacheDuration:     15 * time.Second,
		defaultMarginType: futures.MarginTypeIsolated,
	}
}

// SetDefaultMarginMode 设置默认保证金模式（每次开仓前使用）
func (t *FuturesTrader) SetDefaultMarginMode(mode string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "cross", "crossed", "cross_margin", "crossed_margin":
		t.defaultMarginType = futures.MarginTypeCrossed
		log.Printf("⚙️  默认保证金模式已设置为 CROSS")
	default:
		t.defaultMarginType = futures.MarginTypeIsolated
		log.Printf("⚙️  默认保证金模式已设置为 ISOLATED")
	}
}

// GetBalance 获取账户余额（带缓存）
func (t *FuturesTrader) GetBalance() (map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的账户余额（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用币安API获取账户余额...")
	account, err := t.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		log.Printf("❌ 币安API调用失败: %v", err)
		return nil, fmt.Errorf("获取账户信息失败: %w", err)
	}

	result := make(map[string]interface{})
	result["totalWalletBalance"], _ = strconv.ParseFloat(account.TotalWalletBalance, 64)
	result["availableBalance"], _ = strconv.ParseFloat(account.AvailableBalance, 64)
	result["totalUnrealizedProfit"], _ = strconv.ParseFloat(account.TotalUnrealizedProfit, 64)

	log.Printf("✓ 币安API返回: 总余额=%s, 可用=%s, 未实现盈亏=%s",
		account.TotalWalletBalance,
		account.AvailableBalance,
		account.TotalUnrealizedProfit)

	// 更新缓存
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
}

// GetPositions 获取所有持仓（带缓存）
func (t *FuturesTrader) GetPositions() ([]map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.positionsCacheMutex.RLock()
	if t.cachedPositions != nil && time.Since(t.positionsCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.positionsCacheTime)
		t.positionsCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的持仓信息（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedPositions, nil
	}
	t.positionsCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用币安API获取持仓信息...")
	positions, err := t.client.NewGetPositionRiskService().Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		posAmt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if posAmt == 0 {
			continue // 跳过无持仓的
		}

		posMap := make(map[string]interface{})
		posMap["symbol"] = pos.Symbol
		posMap["positionAmt"], _ = strconv.ParseFloat(pos.PositionAmt, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(pos.EntryPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(pos.MarkPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(pos.UnRealizedProfit, 64)
		posMap["leverage"], _ = strconv.ParseFloat(pos.Leverage, 64)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(pos.LiquidationPrice, 64)

		// 获取保证金模式
		// 币安API的PositionRisk结构体可能不直接包含marginType字段
		// 由于系统在开仓时总是设置为逐仓模式（Isolated），这里默认设为isolated
		// 如果需要从API获取，可以查看PositionRisk的其他字段
		marginType := "isolated" // 默认逐仓（系统统一使用逐仓模式）
		posMap["marginType"] = marginType

		// 判断方向
		if posAmt > 0 {
			posMap["side"] = "long"
		} else {
			posMap["side"] = "short"
		}

		result = append(result, posMap)
	}

	// 更新缓存
	t.positionsCacheMutex.Lock()
	t.cachedPositions = result
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	return result, nil
}

// SetLeverage 设置杠杆（智能判断+冷却期）
func (t *FuturesTrader) SetLeverage(symbol string, leverage int) error {
	// 先尝试获取当前杠杆（从持仓信息）
	currentLeverage := 0
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if lev, ok := pos["leverage"].(float64); ok {
					currentLeverage = int(lev)
					break
				}
			}
		}
	}

	// 如果当前杠杆已经是目标杠杆，跳过
	if currentLeverage == leverage && currentLeverage > 0 {
		log.Printf("  ✓ %s 杠杆已是 %dx，无需切换", symbol, leverage)
		return nil
	}

	// 切换杠杆
	_, err = t.client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(context.Background())

	if err != nil {
		// 如果错误信息包含"No need to change"，说明杠杆已经是目标值
		if contains(err.Error(), "No need to change") {
			log.Printf("  ✓ %s 杠杆已是 %dx", symbol, leverage)
			return nil
		}
		return fmt.Errorf("设置杠杆失败: %w", err)
	}

	log.Printf("  ✓ %s 杠杆已切换为 %dx", symbol, leverage)

	// 切换杠杆后等待5秒（避免冷却期错误）
	log.Printf("  ⏱ 等待5秒冷却期...")
	time.Sleep(5 * time.Second)

	return nil
}

// SetMarginType 设置保证金模式
func (t *FuturesTrader) SetMarginType(symbol string, marginType futures.MarginType) error {
	err := t.client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(marginType).
		Do(context.Background())

	if err != nil {
		// 如果已经是该模式，不算错误
		if contains(err.Error(), "No need to change") {
			log.Printf("  ✓ %s 保证金模式已是 %s", symbol, marginType)
			return nil
		}
		return fmt.Errorf("设置保证金模式失败: %w", err)
	}

	log.Printf("  ✓ %s 保证金模式已切换为 %s", symbol, marginType)

	// 切换保证金模式后等待3秒（避免冷却期错误）
	log.Printf("  ⏱ 等待3秒冷却期...")
	time.Sleep(3 * time.Second)

	return nil
}

// OpenLong 开多仓
func (t *FuturesTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 设置保证金模式（逐仓/全仓）
	if err := t.SetMarginType(symbol, t.defaultMarginType); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开多仓失败: %w", err)
	}

	log.Printf("✓ 开多仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// OpenShort 开空仓
func (t *FuturesTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 设置保证金模式（逐仓/全仓）
	if err := t.SetMarginType(symbol, t.defaultMarginType); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开空仓失败: %w", err)
	}

	log.Printf("✓ 开空仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseLong 平多仓
func (t *FuturesTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity, _ = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的多仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单（平多）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平多仓失败: %w", err)
	}

	log.Printf("✓ 平多仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseShort 平空仓
func (t *FuturesTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				amt, _ := pos["positionAmt"].(float64)
				quantity = -amt // 空仓数量是负的，取绝对值
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的空仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单（平空）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平空仓失败: %w", err)
	}

	log.Printf("✓ 平空仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CancelAllOrders 取消该币种的所有挂单
func (t *FuturesTrader) CancelAllOrders(symbol string) error {
	err := t.client.NewCancelAllOpenOrdersService().
		Symbol(symbol).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("取消挂单失败: %w", err)
	}

	log.Printf("  ✓ 已取消 %s 的所有挂单", symbol)
	return nil
}

// GetMarketPrice 获取市场价格
func (t *FuturesTrader) GetMarketPrice(symbol string) (float64, error) {
	prices, err := t.client.NewListPricesService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取价格失败: %w", err)
	}

	if len(prices) == 0 {
		return 0, fmt.Errorf("未找到价格")
	}

	price, err := strconv.ParseFloat(prices[0].Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// CalculatePositionSize 计算仓位大小
func (t *FuturesTrader) CalculatePositionSize(balance, riskPercent, price float64, leverage int) float64 {
	riskAmount := balance * (riskPercent / 100.0)
	positionValue := riskAmount * float64(leverage)
	quantity := positionValue / price
	return quantity
}

// SetStopLoss 设置止损单
func (t *FuturesTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeStopMarket).
		StopPrice(fmt.Sprintf("%.8f", stopPrice)).
		Quantity(quantityStr).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止损失败: %w", err)
	}

	log.Printf("  止损价设置: %.4f", stopPrice)
	return nil
}

// SetTakeProfit 设置止盈单
func (t *FuturesTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeTakeProfitMarket).
		StopPrice(fmt.Sprintf("%.8f", takeProfitPrice)).
		Quantity(quantityStr).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止盈失败: %w", err)
	}

	log.Printf("  止盈价设置: %.4f", takeProfitPrice)
	return nil
}

// GetOpenOrders 获取指定币种的挂单（包括止盈止损订单）
func (t *FuturesTrader) GetOpenOrders(symbol string) ([]map[string]interface{}, error) {
	orders, err := t.client.NewListOpenOrdersService().
		Symbol(symbol).
		Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取挂单失败: %w", err)
	}

	var result []map[string]interface{}
	for _, order := range orders {
		// 只关注止盈止损订单
		orderType := string(order.Type)
		if orderType != "STOP_MARKET" && orderType != "TAKE_PROFIT_MARKET" {
			continue
		}

		stopPrice, _ := strconv.ParseFloat(order.StopPrice, 64)
		quantity, _ := strconv.ParseFloat(order.OrigQuantity, 64)

		orderMap := map[string]interface{}{
			"symbol":    order.Symbol,
			"orderId":   order.OrderID,
			"side":      string(order.Side),
			"type":      orderType,
			"stopPrice": stopPrice,
			"quantity":  quantity,
		}

		// 如果是止盈单
		if orderType == "TAKE_PROFIT_MARKET" {
			orderMap["orderType"] = "take_profit"
		} else if orderType == "STOP_MARKET" {
			orderMap["orderType"] = "stop_loss"
		}

		result = append(result, orderMap)
	}

	return result, nil
}

// GetSymbolPrecision 获取交易对的数量精度
func (t *FuturesTrader) GetSymbolPrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取交易规则失败: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// 从LOT_SIZE filter获取精度
			for _, filter := range s.Filters {
				if filter["filterType"] == "LOT_SIZE" {
					stepSize := filter["stepSize"].(string)
					precision := calculatePrecision(stepSize)
					log.Printf("  %s 数量精度: %d (stepSize: %s)", symbol, precision, stepSize)
					return precision, nil
				}
			}
		}
	}

	log.Printf("  ⚠ %s 未找到精度信息，使用默认精度3", symbol)
	return 3, nil // 默认精度为3
}

// calculatePrecision 从stepSize计算精度
func calculatePrecision(stepSize string) int {
	// 去除尾部的0
	stepSize = trimTrailingZeros(stepSize)

	// 查找小数点
	dotIndex := -1
	for i := 0; i < len(stepSize); i++ {
		if stepSize[i] == '.' {
			dotIndex = i
			break
		}
	}

	// 如果没有小数点或小数点在最后，精度为0
	if dotIndex == -1 || dotIndex == len(stepSize)-1 {
		return 0
	}

	// 返回小数点后的位数
	return len(stepSize) - dotIndex - 1
}

// trimTrailingZeros 去除尾部的0
func trimTrailingZeros(s string) string {
	// 如果没有小数点，直接返回
	if !stringContains(s, ".") {
		return s
	}

	// 从后向前遍历，去除尾部的0
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}

	// 如果最后一位是小数点，也去掉
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}

	return s
}

// FormatQuantity 格式化数量到正确的精度
func (t *FuturesTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	precision, err := t.GetSymbolPrecision(symbol)
	if err != nil {
		// 如果获取失败，使用默认格式
		return fmt.Sprintf("%.3f", quantity), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, quantity), nil
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetTradeHistory 获取历史成交记录（从币安API获取）
// 改进：支持分页获取，尽可能获取更多历史数据
func (t *FuturesTrader) GetTradeHistory(limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 1000 // 默认最多1000条
	}
	if limit > 1000 {
		limit = 1000 // 币安API单次请求限制最多1000条
	}

	// 币安API限制：最大时间间隔为7天，最多1000条/次
	// 策略：分多个7天时间段获取，每次最多1000条
	endTime := time.Now().Unix() * 1000
	// 获取最近30天的数据（分多个7天时间段）
	daysToFetch := 30
	maxDaysPerRequest := 7 // 币安API限制：最大7天间隔

	var allTrades []*futures.AccountTrade
	currentEndTime := endTime
	maxRequests := 20 // 最多请求20次（30天 ÷ 7天 ≈ 5次，留有余量）

	for i := 0; i < maxRequests && len(allTrades) < limit; i++ {
		// 计算当前请求的时间范围（最多7天）
		currentStartTime := currentEndTime - int64(maxDaysPerRequest*24*3600*1000)
		if currentStartTime < 0 {
			currentStartTime = 0
		}

		// 使用币安合约账户成交API获取历史成交
		trades, err := t.client.NewListAccountTradeService().
			StartTime(currentStartTime).
			EndTime(currentEndTime).
			Limit(1000). // 每次最多1000条
			Do(context.Background())
		if err != nil {
			if i == 0 {
				return nil, fmt.Errorf("获取历史成交失败: %w", err)
			}
			// 如果后续请求失败，返回已获取的数据
			log.Printf("⚠️  获取历史成交（%d-%d）失败: %v，返回已获取的数据", currentStartTime, currentEndTime, err)
			break
		}

		if len(trades) == 0 {
			// 没有更多数据，向前移动时间窗口
			currentEndTime = currentStartTime - 1
			if currentEndTime <= 0 {
				break // 已经到达最早时间
			}
			continue
		}

		// 将新获取的数据添加到结果中（注意：币安API返回的是从旧到新，需要合并）
		allTrades = append(allTrades, trades...)

		// 如果返回的数据少于1000条，说明这个时间段已经获取完所有数据
		if len(trades) < 1000 {
			// 向前移动时间窗口，继续获取更早的数据
			currentEndTime = currentStartTime - 1
			if currentEndTime <= 0 {
				break // 已经到达最早时间
			}
			continue
		}

		// 如果返回了1000条，可能还有更多数据，需要在这个时间段内继续分页
		// 但币安API限制时间间隔最大7天，所以我们需要按时间分页
		// 找到这个时间段内的最早交易时间，作为下次请求的结束时间
		if len(trades) > 0 {
			earliestTradeTime := trades[0].Time // 第一条是最早的
			if earliestTradeTime <= currentStartTime {
				// 已经获取完这个时间段的所有数据，向前移动时间窗口
				currentEndTime = currentStartTime - 1
				if currentEndTime <= 0 {
					break
				}
			} else {
				// 在这个时间段内继续分页
				currentEndTime = earliestTradeTime - 1
			}
		}

		// 如果已经获取足够的数据，停止
		if len(allTrades) >= limit {
			allTrades = allTrades[:limit]
			break
		}

		// 检查是否已经获取了足够天数的数据
		earliestTime := endTime
		for _, t := range allTrades {
			if t.Time < earliestTime {
				earliestTime = t.Time
			}
		}
		daysFetched := (endTime - earliestTime) / (24 * 3600 * 1000)
		if daysFetched >= int64(daysToFetch) {
			break // 已经获取了足够天数的数据
		}
	}

	trades := allTrades

	result := make([]map[string]interface{}, 0, len(trades))
	for _, trade := range trades {
		// 解析价格和数量
		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.Quantity, 64)
		realizedPnl, _ := strconv.ParseFloat(trade.RealizedPnl, 64)
		commission, _ := strconv.ParseFloat(trade.Commission, 64)

		// 判断方向：买单为开多/平空，卖单为开空/平多
		side := "long"
		action := "open_long"
		if trade.Side == futures.SideTypeSell {
			side = "short"
			action = "open_short"
		}

		// 如果有realized_pnl，说明是平仓
		if realizedPnl != 0 {
			if trade.Side == futures.SideTypeBuy {
				action = "close_short"
			} else {
				action = "close_long"
			}
		}

		// 解析时间戳
		timeUnix := trade.Time / 1000 // 转换为秒

		result = append(result, map[string]interface{}{
			"order_id":     trade.OrderID,
			"symbol":       trade.Symbol,
			"side":         side,
			"action":       action,
			"price":        price,
			"quantity":     qty,
			"realized_pnl": realizedPnl,
			"commission":   commission,
			"time":         timeUnix,
			"time_millis":  trade.Time,
			"source":       "exchange", // 标记为交易所数据
		})
	}

	// 按时间倒序排列（最新的在前）
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}
