#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
AI新闻量化交易系统管理器
整合配置编辑 + 提示词编辑
"""

import json
import os
import sys
import tempfile
import subprocess
import datetime

# =============================================================================
# 配置编辑模块
# =============================================================================

CONFIG_FILE = "config.json"
CONFIG_EXAMPLE = "config.json.example"
DEFAULT_COIN_PRESET = [
    "BTCUSDT",
    "ETHUSDT",
    "SOLUSDT",
    "BNBUSDT",
    "XRPUSDT",
    "DOGEUSDT",
    "ADAUSDT",
    "AVAXUSDT",
    "TRXUSDT",
    "LINKUSDT",
    "DOTUSDT",
    "ATOMUSDT",
    "LTCUSDT",
    "ARBUSDT",
    "OPUSDT",
    "HYPEUSDT",
]

def load_config():
    """加载配置文件"""
    if os.path.exists(CONFIG_FILE):
        with open(CONFIG_FILE, 'r', encoding='utf-8') as f:
            return json.load(f)
    elif os.path.exists(CONFIG_EXAMPLE):
        print("⚠️  config.json 不存在，已从模板复制")
        with open(CONFIG_EXAMPLE, 'r', encoding='utf-8') as f:
            return json.load(f)
    else:
        print("❌ 找不到配置文件")
        return None

def save_config(config):
    """保存配置文件"""
    with open(CONFIG_FILE, 'w', encoding='utf-8') as f:
        json.dump(config, f, indent=2, ensure_ascii=False)
    print(f"✅ 配置已保存到 {CONFIG_FILE}")

def normalize_coin_symbol(symbol):
    """规范化币种符号，默认补全USDT后缀"""
    cleaned = symbol.strip().upper().replace("/", "").replace("-", "").replace("_", "")
    if not cleaned:
        return ""

    suffixes = ("USDT", "USDC", "BUSD", "FDUSD", "USD")
    if not cleaned.endswith(suffixes):
        cleaned += "USDT"
    return cleaned

def parse_coin_list(raw_value):
    """解析用户输入的币种列表，保留顺序并去重"""
    tokens = raw_value.replace("\n", ",").replace(" ", ",").split(",")
    result = []
    seen = set()

    for token in tokens:
        symbol = normalize_coin_symbol(token)
        if not symbol or symbol in seen:
            continue
        seen.add(symbol)
        result.append(symbol)

    return result

def edit_trader(config):
    """编辑交易员配置"""
    traders = config.get('traders', [])
    
    print("\n📊 交易员列表:")
    for i, trader in enumerate(traders):
        status = "✅ 启用" if trader.get('enabled', False) else "❌ 禁用"
        print(f"  [{i}] {trader.get('name', '未命名')} ({trader.get('ai_model', '未知')}) - {status}")
    
    choice = input("\n选择要编辑的交易员编号 [0-{}]: ".format(len(traders)-1))
    try:
        idx = int(choice)
        trader = traders[idx]
    except (ValueError, IndexError):
        print("❌ 无效选择")
        return False
    
    print(f"\n📝 正在编辑: {trader.get('name')}")
    print("-" * 40)
    
    fields = [
        ('name', '交易员名称', str),
        ('ai_model', 'AI 模型 (deepseek/qwen)', str),
        ('exchange', '交易所 (binance)', str),
        ('binance_api_key', 'Binance API Key', str),
        ('binance_secret_key', 'Binance Secret Key', str),
        ('deepseek_key', 'DeepSeek API Key', str),
        ('qwen_key', 'Qwen API Key', str),
        ('initial_balance', '初始余额 (USDT)', float),
        ('scan_interval_minutes', '扫描间隔 (分钟)', int),
    ]
    
    for field, desc, field_type in fields:
        current = trader.get(field, '')
        if 'key' in field.lower() and current:
            display = current[:10] + "..." if len(current) > 10 else current
        else:
            display = current
        
        new_value = input(f"{desc} [{display}]: ").strip()
        if new_value:
            try:
                trader[field] = field_type(new_value)
            except ValueError:
                print(f"⚠️  输入格式错误，保留原值")
    
    enabled = trader.get('enabled', False)
    enable_input = input(f"启用该交易员? [{'Y' if enabled else 'N'}]: ").strip().lower()
    if enable_input:
        trader['enabled'] = enable_input in ['y', 'yes', '是', '1']
    
    print(f"✅ 交易员 {trader.get('name')} 更新完成")
    return True

def edit_leverage(config):
    """编辑杠杆配置"""
    print("\n📈 杠杆配置:")
    leverage = config.get('leverage', {})
    
    btc_leverage = leverage.get('btc_eth_leverage', 5)
    new_btc = input(f"BTC/ETH 杠杆上限 [{btc_leverage}]: ").strip()
    if new_btc:
        leverage['btc_eth_leverage'] = int(new_btc)
    
    alt_leverage = leverage.get('altcoin_leverage', 5)
    new_alt = input(f"山寨币杠杆上限 [{alt_leverage}]: ").strip()
    if new_alt:
        leverage['altcoin_leverage'] = int(new_alt)
    
    print("✅ 杠杆配置更新完成")
    return True

def edit_news(config):
    """编辑新闻配置"""
    print("\n📰 新闻配置:")
    
    enabled = config.get('news_opennews_enabled', False)
    enable_input = input(f"启用 OpenNews? [{'Y' if enabled else 'N'}]: ").strip().lower()
    if enable_input:
        config['news_opennews_enabled'] = enable_input in ['y', 'yes', '是', '1']
    
    api_key = config.get('news_opennews_api_key', '')
    new_key = input(f"OpenNews API Key [{api_key[:10] if api_key else '未设置'}...]: ").strip()
    if new_key:
        config['news_opennews_api_key'] = new_key
    
    print("✅ 新闻配置更新完成")
    return True

def edit_risk(config):
    """编辑风控参数"""
    print("\n🛡️  风控配置:")
    
    max_daily_loss = config.get('max_daily_loss', 10.0)
    new_loss = input(f"单日最大亏损 (%) [{max_daily_loss}]: ").strip()
    if new_loss:
        config['max_daily_loss'] = float(new_loss)
    
    max_drawdown = config.get('max_drawdown', 20.0)
    new_dd = input(f"最大回撤 (%) [{max_drawdown}]: ").strip()
    if new_dd:
        config['max_drawdown'] = float(new_dd)
    
    stop_minutes = config.get('stop_trading_minutes', 60)
    new_stop = input(f"触发风控后暂停交易 (分钟) [{stop_minutes}]: ").strip()
    if new_stop:
        config['stop_trading_minutes'] = int(new_stop)
    
    print("✅ 风控配置更新完成")
    return True

def edit_coin_pool(config):
    """编辑币种池配置"""
    while True:
        use_default = config.get('use_default_coins', True)
        default_coins = config.get('default_coins', []) or DEFAULT_COIN_PRESET.copy()
        coin_pool_api_url = config.get('coin_pool_api_url', '')

        print("\n" + "="*40)
        print("  🪙 币种池配置")
        print("="*40)
        print(f"当前模式: {'白名单优先' if use_default else 'AI500/API优先'}")
        print(f"白名单币种数: {len(default_coins)}")
        print(f"AI500 API: {coin_pool_api_url or '未设置'}")
        print("-"*40)
        print("1. 👁️  查看当前白名单币种")
        print("2. 🔁 切换 use_default_coins")
        print("3. ✏️  编辑白名单币种")
        print("4. 🌐 编辑 AI500 API URL")
        print("5. ♻️  恢复推荐白名单")
        print("6. ⬅️  返回")
        print("="*40)

        choice = input("\n请选择 [1-6]: ").strip()

        if choice == '1':
            print("\n📋 当前白名单币种:")
            for i, symbol in enumerate(default_coins, start=1):
                print(f"  {i:>2}. {symbol}")
            input("\n按回车继续...")
        elif choice == '2':
            config['use_default_coins'] = not use_default
            status = "启用" if config['use_default_coins'] else "关闭"
            print(f"✅ 已{status}白名单优先模式")
        elif choice == '3':
            print("\n当前白名单:")
            print(", ".join(default_coins))
            print("💡 支持逗号、空格或换行分隔；输入 BTC 会自动补成 BTCUSDT")
            raw_value = input("请输入新的币种列表: ").strip()
            if raw_value:
                symbols = parse_coin_list(raw_value)
                if symbols:
                    config['default_coins'] = symbols
                    print(f"✅ 白名单已更新，共 {len(symbols)} 个币种")
                else:
                    print("⚠️  未解析出有效币种，保留原值")
        elif choice == '4':
            new_url = input(f"AI500 API URL [{coin_pool_api_url or '未设置'}]: ").strip()
            if new_url:
                config['coin_pool_api_url'] = new_url
            elif coin_pool_api_url:
                clear = input("清空 AI500 API URL? [y/N]: ").strip().lower()
                if clear in ['y', 'yes', '是', '1']:
                    config['coin_pool_api_url'] = ''
            print("✅ AI500 API 配置已更新")
        elif choice == '5':
            config['default_coins'] = DEFAULT_COIN_PRESET.copy()
            print(f"✅ 已恢复推荐白名单，共 {len(config['default_coins'])} 个币种")
        elif choice == '6':
            return True
        else:
            print("❌ 无效选择")

def config_menu(config):
    """配置编辑菜单"""
    modified = False
    
    while True:
        print("\n" + "="*40)
        print("  ⚙️  配置编辑器")
        print("="*40)
        print("1. 🧑‍💼 编辑交易员配置")
        print("2. 📈 编辑杠杆配置")
        print("3. 📰 编辑新闻配置")
        print("4. 🪙 编辑币种池配置")
        print("5. 🛡️  编辑风控配置")
        print("6. 💾 保存并返回")
        print("7. ⬅️  返回（不保存）")
        print("="*40)
        
        choice = input("\n请选择 [1-7]: ").strip()
        
        if choice == '1':
            if edit_trader(config):
                modified = True
        elif choice == '2':
            if edit_leverage(config):
                modified = True
        elif choice == '3':
            if edit_news(config):
                modified = True
        elif choice == '4':
            if edit_coin_pool(config):
                modified = True
        elif choice == '5':
            if edit_risk(config):
                modified = True
        elif choice == '6':
            save_config(config)
            print("\n💡 提示: 配置已保存，需要重启系统才能生效")
            restart = input("是否立即重启系统? [y/N]: ").strip().lower()
            if restart in ['y', 'yes', '是']:
                restart_system()
            return
        elif choice == '7':
            if modified:
                confirm = input("⚠️  有未保存的更改，确定返回? [y/N]: ").strip().lower()
                if confirm in ['y', 'yes']:
                    return
            else:
                return
        else:
            print("❌ 无效选择")

# =============================================================================
# 提示词编辑模块
# =============================================================================

PROMPT_FILE = "decision/templates/system_prompt.tmpl"
BACKUP_DIR = "decision/templates/backups"

def load_prompt():
    """加载提示词文件"""
    if not os.path.exists(PROMPT_FILE):
        print(f"❌ 找不到提示词文件: {PROMPT_FILE}")
        return None
    
    with open(PROMPT_FILE, 'r', encoding='utf-8') as f:
        return f.read()

def save_prompt(content):
    """保存提示词文件"""
    if not os.path.exists(BACKUP_DIR):
        os.makedirs(BACKUP_DIR)
    
    backup_name = f"system_prompt_{datetime.datetime.now().strftime('%Y%m%d_%H%M%S')}.tmpl"
    backup_path = os.path.join(BACKUP_DIR, backup_name)
    
    if os.path.exists(PROMPT_FILE):
        with open(PROMPT_FILE, 'r', encoding='utf-8') as f:
            old_content = f.read()
        with open(backup_path, 'w', encoding='utf-8') as f:
            f.write(old_content)
        print(f"📦 已创建备份: {backup_path}")
    
    with open(PROMPT_FILE, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"✅ 提示词已保存到 {PROMPT_FILE}")

def rebuild_backend():
    """重建并重启后端，使 go:embed 的提示词修改生效"""
    docker_cmd = get_docker_compose_cmd()

    if docker_cmd is None:
        print("❌ 错误: 未检测到 Docker Compose")
        print("💡 提示词已保存，但后端需要重新构建后才能生效")
        return False

    print("🔨 正在重建并重启 backend（提示词通过 go:embed 编译进二进制）...")
    result = os.system(f"{docker_cmd} up -d --build backend")
    if result == 0:
        print("✅ backend 已重建并重启，最新提示词已生效")
        return True

    print("❌ backend 重建失败")
    return False

def rebuild_system():
    """全量重建并重启系统"""
    docker_cmd = get_docker_compose_cmd()

    if docker_cmd is None:
        print("❌ 错误: 未检测到 Docker Compose")
        return False

    print("🔨 正在全量重建并重启系统...")
    result = os.system(f"{docker_cmd} up -d --build")
    if result == 0:
        print("✅ 系统已全量重建并重启")
        return True

    print("❌ 系统全量重建失败")
    return False

def edit_with_editor(content):
    """使用系统编辑器编辑"""
    # 优先使用 nano（更容易上手），其次 vim，最后 vi
    if os.environ.get('EDITOR'):
        editor = os.environ['EDITOR']
    elif subprocess.call(['which', 'nano'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) == 0:
        editor = 'nano'
    elif subprocess.call(['which', 'vim'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) == 0:
        editor = 'vim'
    else:
        editor = 'vi'
    
    print(f"\n📋 将使用 {editor} 编辑器打开提示词文件...")
    if editor in ['vim', 'vi']:
        print("💡 vim 退出方法: 按 Esc 键，然后输入 :wq 回车（保存）或 :q! 回车（不保存）")
    elif editor == 'nano':
        print("💡 nano 退出方法: 按 Ctrl+O 回车保存，然后按 Ctrl+X 退出")
    print("")
    
    with tempfile.NamedTemporaryFile(mode='w+', suffix='.tmpl', delete=False) as tmp:
        tmp.write(content)
        tmp_path = tmp.name
    
    try:
        subprocess.call([editor, tmp_path])
        with open(tmp_path, 'r', encoding='utf-8') as f:
            return f.read()
    finally:
        os.unlink(tmp_path)

def get_default_template():
    """获取默认模板"""
    return '''你是一个加密货币量化交易助手，每3分钟分析一次市场并输出交易决策。

## 核心目标
- 在控制回撤的前提下，持续提升账户净值，优先追求风险调整后的稳定收益
- 优先选择赔率清晰、逻辑完整、执行后胜率和盈亏比更优的机会
- 先处理已有持仓，再决定是否开新仓；避免无效折腾，但也不要因为犹豫长期空仓
- 若当前没有足够优势，就明确输出 `hold` 或 `wait`，不要为了交易而交易

## 账户
当前净值：{{.AccountEquity}} USDT

## 账户与仓位边界
- 最多持仓3个币种
- 总持仓占比（总名义仓位 / 账户净值）≤ 200%
- BTC/ETH：单笔仓位 {{.BtcRangeLow}}-{{.BtcRangeHigh}} USDT，杠杆 1-{{.BtcEthLeverage}}x
- 山寨币：单笔仓位 {{.AltRangeLow}}-{{.AltRangeHigh}} USDT，杠杆 1-{{.AltcoinLeverage}}x

## 交易原则
- 风险回报比目标 ≥ 1:{{.MinRiskRewardRatio}}
- 所有涨跌幅、止盈止损判断都按未乘杠杆的标的价格计算
- 做多和做空是对等工具，不允许主观方向偏见
- 技术面、资金面、结构位置、新闻与宏观环境要综合判断，不能只看单一指标
- 资金费率、OI、成交量、趋势斜率、支撑阻力、波动率都可以作为决策依据

## 执行目标
- 开仓前给出明确方向、仓位、杠杆、止损、止盈和信心度
- 尽量寻找赔率清晰的机会，而不是模糊区间内勉强出手
- 如果已有持仓接近止盈/止损、逻辑被破坏、或出现更优换仓机会，优先调整已有仓位
- 如果市场结构混乱、信号冲突严重、或胜率边际不足，可以观望

## 决策偏好
- 趋势明确时，可顺势交易并适当提高信心度
- 极端超卖/超买、资金费率异常、或关键支撑阻力附近，可以考虑反转交易，但必须说明触发依据
- 若新闻和宏观环境明显偏多/偏空，应把它纳入最终决策，而不是只看K线
- 输出应尽量具体，避免空泛理由，如“感觉会涨”“可能反弹”

## 输出要求
先写思维链分析（纯文本，不要出现方括号 `[` 或 `]`），然后输出 JSON 决策数组，必须包裹在 ```json 代码块中：

```json
[
  {"symbol": "BTCUSDT", "action": "open_short", "leverage": {{.SampleBTCLeverage}}, "position_size_usd": {{.SamplePositionUSD}}, "stop_loss": 3500, "take_profit": 3450, "confidence": 68, "risk_usd": 200, "reasoning": "..."}
]
```

字段说明：
- action: open_long | open_short | close_long | close_short | hold | wait
- 开仓必填: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd, reasoning
- 平仓必填: position_id, reasoning
- 禁止输出未列出的 action

## 额外要求
- 若开仓，`reasoning` 必须说明为什么现在比继续等待更优
- 若平仓，`reasoning` 必须说明是止盈、止损、换仓还是逻辑失效
- 若观望，明确说明缺少什么条件，下一步等待什么信号'''

def list_backups():
    """列出备份文件"""
    if not os.path.exists(BACKUP_DIR):
        print("📂 没有备份文件")
        return []
    
    backups = sorted([f for f in os.listdir(BACKUP_DIR) if f.endswith('.tmpl')])
    if not backups:
        print("📂 没有备份文件")
        return []
    
    print("\n📦 备份列表:")
    for i, backup in enumerate(backups):
        print(f"  [{i}] {backup}")
    
    return backups

def restore_backup():
    """恢复备份"""
    backups = list_backups()
    if not backups:
        return
    
    choice = input("\n选择要恢复的备份编号: ").strip()
    try:
        idx = int(choice)
        backup_name = backups[idx]
        backup_path = os.path.join(BACKUP_DIR, backup_name)
        
        with open(backup_path, 'r', encoding='utf-8') as f:
            content = f.read()
        
        save_prompt(content)
        print(f"✅ 已恢复备份: {backup_name}")
        print("💡 恢复后的提示词需重建 backend 才会生效")
    except (ValueError, IndexError):
        print("❌ 无效选择")

def prompt_menu():
    """提示词编辑菜单"""
    if not os.path.exists(PROMPT_FILE):
        print(f"⚠️  提示词文件不存在，创建默认模板...")
        os.makedirs(os.path.dirname(PROMPT_FILE), exist_ok=True)
        content = get_default_template()
        with open(PROMPT_FILE, 'w', encoding='utf-8') as f:
            f.write(content)
    else:
        content = load_prompt()
    
    if content is None:
        return
    
    modified = False
    
    while True:
        print("\n" + "="*40)
        print("  📝 提示词编辑器")
        print("="*40)
        print("1. ✏️  使用编辑器编辑")
        print("2. 👁️  查看当前提示词")
        print("3. 📦 查看备份列表")
        print("4. 🔄 恢复备份")
        print("5. 💾 保存并返回")
        print("6. ⬅️  返回（不保存）")
        print("="*40)
        
        choice = input("\n请选择 [1-6]: ").strip()
        
        if choice == '1':
            new_content = edit_with_editor(content)
            if new_content != content:
                content = new_content
                modified = True
                print("✅ 提示词已修改")
        elif choice == '2':
            print("\n" + "="*60)
            print(content)
            print("="*60)
            input("\n按回车继续...")
        elif choice == '3':
            list_backups()
        elif choice == '4':
            restore_backup()
            content = load_prompt()
        elif choice == '5':
            save_prompt(content)
            print("💡 提示: system_prompt.tmpl 通过 go:embed 编译进 backend，普通 restart 不会加载新提示词")
            print("1. 立即重建 backend（推荐）")
            print("2. 全量重建前后端")
            print("3. 仅保存，稍后手动处理")
            rebuild_choice = input("请选择 [1-3]: ").strip()
            if rebuild_choice == '1':
                rebuild_backend()
            elif rebuild_choice == '2':
                rebuild_system()
            return
        elif choice == '6':
            if modified:
                confirm = input("⚠️  有未保存的更改，确定返回? [y/N]: ").strip().lower()
                if confirm in ['y', 'yes']:
                    return
            else:
                return
        else:
            print("❌ 无效选择")

# =============================================================================
# 主菜单
# =============================================================================

def show_main_menu():
    """显示主菜单"""
    print("\n" + "="*50)
    print("  🤖 AI新闻量化交易系统管理器")
    print("="*50)
    print("1. ⚙️  编辑系统配置 (config.json)")
    print("2. 📝 编辑交易提示词 (System Prompt)")
    print("3. 🚀 启动系统")
    print("4. ❌ 退出")
    print("="*50)

def get_docker_compose_cmd():
    """检测 docker compose 命令格式"""
    # 检查新版 docker compose (Docker 20.10+)
    result = os.system("docker compose version > /dev/null 2>&1")
    if result == 0:
        return "docker compose"
    
    # 检查旧版 docker-compose
    result = os.system("docker-compose version > /dev/null 2>&1")
    if result == 0:
        return "docker-compose"
    
    return None

def start_system():
    """启动系统"""
    print("\n🚀 启动选项:")
    print("1. 启动后端 (./main)")
    print("2. 启动前端 (npm run dev)")
    print("3. 启动前后端 (Docker Compose)")
    print("4. 返回")
    
    choice = input("\n请选择 [1-4]: ").strip()
    
    if choice == '1':
        print("🚀 启动后端...")
        os.system("./main")
    elif choice == '2':
        print("🚀 启动前端...")
        os.system("cd web && npm run dev")
    elif choice == '3':
        docker_cmd = get_docker_compose_cmd()
        if docker_cmd is None:
            print("❌ 错误: 未检测到 Docker Compose")
            print("💡 请确保 Docker 已安装")
            print("   安装方法:")
            print("   - Ubuntu/Debian: sudo apt install docker-compose-plugin")
            print("   - 或使用: ./install.sh 重新安装 Docker")
            return
        
        print(f"🚀 启动 Docker Compose ({docker_cmd})...")
        result = os.system(f"{docker_cmd} up -d")
        if result == 0:
            print("\n" + "="*50)
            print("  ✅ 系统启动成功!")
            print("="*50)
            print("\n📊 访问面板:")
            print("   本地访问: http://localhost:3000")
            print("   公网访问: http://<服务器IP>:3000")
            print("\n📋 常用命令:")
            print("   查看日志: docker compose logs -f")
            print("   停止系统: docker compose down")
            print("   重启系统: docker compose restart")
            print("="*50)
    else:
        return

def system_maintenance():
    """系统维护"""
    print("\n🔧 系统维护:")
    print("1. 查看系统状态")
    print("2. 查看日志")
    print("3. 重启系统")
    print("4. 重建 backend")
    print("5. 全量重建系统")
    print("6. 停止系统")
    print("7. 返回")
    
    choice = input("\n请选择 [1-7]: ").strip()
    
    docker_cmd = get_docker_compose_cmd()
    
    if choice == '1':
        print("\n📊 系统状态:")
        if docker_cmd:
            os.system(f"{docker_cmd} ps")
        else:
            print("Docker Compose 未安装，无法查看容器状态")
            print("\n手动检查:")
            os.system("ps aux | grep './main' | grep -v grep")
            os.system("ps aux | grep 'npm run dev' | grep -v grep")
    elif choice == '2':
        print("\n📋 查看日志:")
        print("1. 后端日志")
        print("2. 前端日志")
        print("3. 全部日志")
        log_choice = input("请选择 [1-3]: ").strip()
        if docker_cmd:
            if log_choice == '1':
                os.system(f"{docker_cmd} logs -f backend")
            elif log_choice == '2':
                os.system(f"{docker_cmd} logs -f frontend")
            else:
                os.system(f"{docker_cmd} logs -f")
        else:
            print("Docker 未运行，查看本地日志:")
            if os.path.exists("logs/app.log"):
                os.system("tail -f logs/app.log")
            else:
                print("未找到日志文件")
    elif choice == '3':
        print("🔄 重启系统...")
        if docker_cmd:
            os.system(f"{docker_cmd} restart")
            print("✅ 系统已重启")
        else:
            print("Docker 未运行，请手动重启")
    elif choice == '4':
        rebuild_backend()
    elif choice == '5':
        rebuild_system()
    elif choice == '6':
        print("🛑 停止系统...")
        if docker_cmd:
            os.system(f"{docker_cmd} down")
            print("✅ 系统已停止")
        else:
            print("Docker 未运行")

def restart_system():
    """重启系统"""
    docker_cmd = get_docker_compose_cmd()
    
    if docker_cmd is None:
        print("❌ 错误: 未检测到 Docker Compose")
        print("💡 请确保 Docker 已安装")
        return
    
    print("🔄 正在重启系统...")
    result = os.system(f"{docker_cmd} restart")
    
    if result == 0:
        print("\n" + "="*50)
        print("  ✅ 系统重启成功!")
        print("="*50)
        print("\n⚠️  注意: 若刚修改过提示词，仅 restart 不会生效，请改用“重建 backend”")
        print("\n📊 访问面板:")
        print("   本地访问: http://localhost:3000")
        print("   公网访问: http://<服务器IP>:3000")
        print("="*50)
    else:
        print("❌ 重启失败")

def show_main_menu():
    """显示主菜单"""
    print("\n" + "="*50)
    print("  🤖 AI新闻量化交易系统管理器")
    print("="*50)
    print("1. ⚙️  编辑系统配置 (config.json)")
    print("2. 📝 编辑交易提示词 (System Prompt)")
    print("3. 🚀 启动系统")
    print("4. 🔄 重启系统")
    print("5. 🔧 系统维护")
    print("6. ❌ 退出")
    print("="*50)

def main():
    print("🤖 AI新闻量化交易系统管理器")
    print("="*50)
    
    while True:
        show_main_menu()
        choice = input("\n请选择 [1-6]: ").strip()
        
        if choice == '1':
            config = load_config()
            if config:
                config_menu(config)
        elif choice == '2':
            prompt_menu()
        elif choice == '3':
            start_system()
        elif choice == '4':
            restart_system()
        elif choice == '5':
            system_maintenance()
        elif choice == '6':
            print("\n👋 再见!")
            break
        else:
            print("❌ 无效选择")

if __name__ == '__main__':
    main()
