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
    
    interval = config.get('scan_interval_minutes', 3)
    new_interval = input(f"扫描间隔 (分钟) [{interval}]: ").strip()
    if new_interval:
        config['scan_interval_minutes'] = int(new_interval)
    
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
        print("4. 🛡️  编辑风控配置")
        print("5. 💾 保存并返回")
        print("6. ⬅️  返回（不保存）")
        print("="*40)
        
        choice = input("\n请选择 [1-6]: ").strip()
        
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
            if edit_risk(config):
                modified = True
        elif choice == '5':
            save_config(config)
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

def edit_with_editor(content):
    """使用系统编辑器编辑"""
    editor = os.environ.get('EDITOR', 'vim')
    
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
    return '''# 交易规则

你是一名专业的加密货币量化交易员。你的任务是根据提供的市场数据生成交易决策。

## 核心规则

1. **风险控制优先**
   - 山寨币杠杆 ≤ {{.AltcoinLeverage}}x
   - BTC/ETH 杠杆 ≤ {{.BTCEthLeverage}}x
   - 风险回报比 ≥ 3:1

2. **决策格式**
   ```json
   {
     "symbol": "BTCUSDT",
     "action": "open_long",
     "leverage": 5,
     "position_size_usd": 100,
     "stop_loss": 45000,
     "take_profit": 55000,
     "confidence": 85
   }
   ```

3. **禁止行为**
   - 不得逆向操作（止损>入场价做多）
   - 不得过度杠杆
   - 不得在流动性不足的币种上开仓

## 分析框架

1. 技术面：K线形态、支撑阻力、指标信号
2. 基本面：新闻情绪、宏观环境
3. 资金管理：仓位控制、风险分散

请基于以上规则和数据做出决策。'''

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
        print("🚀 启动 Docker Compose...")
        os.system("docker-compose up -d")
    else:
        return

def main():
    print("🤖 AI新闻量化交易系统管理器")
    print("="*50)
    
    while True:
        show_main_menu()
        choice = input("\n请选择 [1-4]: ").strip()
        
        if choice == '1':
            config = load_config()
            if config:
                config_menu(config)
        elif choice == '2':
            prompt_menu()
        elif choice == '3':
            start_system()
        elif choice == '4':
            print("\n👋 再见!")
            break
        else:
            print("❌ 无效选择")

if __name__ == '__main__':
    main()
