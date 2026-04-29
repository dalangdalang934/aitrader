#!/bin/bash
# =============================================================================
# Trojan代理配置脚本
# 用于服务器访问币安等被限制的API
# =============================================================================

set -e

PROXY_SERVER="175.178.125.122"
PROXY_PORT="4002"
PROXY_PASSWORD="9019244c-cbba-36db-becd-3a23b42b5604"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Trojan代理配置${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 创建trojan配置文件
mkdir -p /etc/trojan

cat > /etc/trojan/config.json << EOF
{
    "run_type": "client",
    "local_addr": "127.0.0.1",
    "local_port": 1080,
    "remote_addr": "${PROXY_SERVER}",
    "remote_port": ${PROXY_PORT},
    "password": [
        "${PROXY_PASSWORD}"
    ],
    "log_level": 1,
    "ssl": {
        "verify": false,
        "verify_hostname": false,
        "cert": "",
        "cipher": "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-AES256-SHA:ECDHE-RSA-AES256-SHA:ECDHE-ECDSA-AES128-SHA:ECDHE-RSA-AES128-SHA:AES256-GCM-SHA384:AES128-GCM-SHA256:AES256-SHA256:AES128-SHA256:AES256-SHA:AES128-SHA",
        "cipher_tls13": "TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_256_GCM_SHA384",
        "sni": "",
        "alpn": [
            "h2",
            "http/1.1"
        ],
        "reuse_session": true,
        "session_ticket": false,
        "curves": ""
    },
    "tcp": {
        "no_delay": true,
        "keep_alive": true,
        "reuse_port": false,
        "fast_open": false,
        "fast_open_qlen": 20
    }
}
EOF

# 安装trojan
echo -e "${YELLOW}正在安装 Trojan 客户端...${NC}"

if ! command -v trojan &> /dev/null; then
    # 下载trojan
    wget -q https://github.com/trojan-gfw/trojan/releases/download/v1.16.0/trojan-1.16.0-linux-amd64.tar.xz
    tar -xf trojan-1.16.0-linux-amd64.tar.xz
    mv trojan/trojan /usr/local/bin/
    rm -rf trojan trojan-1.16.0-linux-amd64.tar.xz
    echo -e "${GREEN}Trojan 安装完成${NC}"
else
    echo -e "${GREEN}Trojan 已安装${NC}"
fi

# 创建systemd服务
cat > /etc/systemd/system/trojan.service << EOF
[Unit]
Description=Trojan Proxy Client
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/trojan -c /etc/trojan/config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 启动trojan
systemctl daemon-reload
systemctl enable trojan
systemctl start trojan

# 等待trojan启动
sleep 2

# 检查trojan是否运行
if systemctl is-active --quiet trojan; then
    echo -e "${GREEN}✅ Trojan 代理已启动${NC}"
    echo -e "${BLUE}代理地址: socks5://127.0.0.1:1080${NC}"
else
    echo -e "${RED}❌ Trojan 启动失败${NC}"
    exit 1
fi

# 设置系统代理环境变量
echo -e "${YELLOW}设置系统代理环境变量...${NC}"

# 添加到 ~/.bashrc
if ! grep -q "PROXY" ~/.bashrc; then
    cat >> ~/.bashrc << 'EOF'

# Trojan代理设置
export HTTP_PROXY=socks5://127.0.0.1:1080
export HTTPS_PROXY=socks5://127.0.0.1:1080
export http_proxy=socks5://127.0.0.1:1080
export https_proxy=socks5://127.0.0.1:1080
# 币安API需要走代理
export BINANCE_PROXY=socks5://127.0.0.1:1080
EOF
    echo -e "${GREEN}环境变量已添加到 ~/.bashrc${NC}"
fi

# 立即生效
export HTTP_PROXY=socks5://127.0.0.1:1080
export HTTPS_PROXY=socks5://127.0.0.1:1080
export http_proxy=socks5://127.0.0.1:1080
export https_proxy=socks5://127.0.0.1:1080
export BINANCE_PROXY=socks5://127.0.0.1:1080

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Trojan代理配置完成！${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}代理信息:${NC}"
echo -e "  协议: SOCKS5"
echo -e "  地址: 127.0.0.1:1080"
echo -e "  服务器: ${PROXY_SERVER}:${PROXY_PORT}"
echo ""
echo -e "${BLUE}使用方法:${NC}"
echo -e "  1. 重启终端或运行: source ~/.bashrc"
echo -e "  2. 测试代理: curl -x socks5://127.0.0.1:1080 https://api.binance.com/api/v3/ping"
echo -e "  3. Docker容器需要额外配置（见下方说明）"
echo ""
echo -e "${YELLOW}Docker容器使用代理:${NC}"
echo -e "  修改 docker-compose.yml，在 backend 服务下添加:${NC}"
echo -e "    environment:${NC}"
echo -e "      - HTTP_PROXY=socks5://host.docker.internal:1080${NC}"
echo -e "      - HTTPS_PROXY=socks5://host.docker.internal:1080${NC}"
echo ""
