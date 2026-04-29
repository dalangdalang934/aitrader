#!/bin/bash
# =============================================================================
# AI新闻量化交易系统 - 一键环境安装脚本
# 支持 macOS / Linux (Ubuntu/Debian/CentOS)
# =============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  AI新闻量化交易系统 - 环境安装脚本${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 检测操作系统
OS=""
if [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macos"
    echo -e "${GREEN}检测到操作系统: macOS${NC}"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="linux"
    echo -e "${GREEN}检测到操作系统: Linux${NC}"
else
    echo -e "${RED}不支持的操作系统: $OSTYPE${NC}"
    exit 1
fi

# 检查命令是否存在
command_exists() {
    command -v "$1" &> /dev/null
}

# 安装 Homebrew (macOS)
install_homebrew() {
    if ! command_exists brew; then
        echo -e "${YELLOW}正在安装 Homebrew...${NC}"
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        echo -e "${GREEN}Homebrew 安装完成${NC}"
    else
        echo -e "${GREEN}Homebrew 已安装${NC}"
    fi
}

# 安装 Go
install_go() {
    if command_exists go; then
        GO_VERSION=$(go version | awk '{print $3}')
        echo -e "${GREEN}Go 已安装: $GO_VERSION${NC}"
        return
    fi
    
    echo -e "${YELLOW}正在安装 Go 1.25+...${NC}"
    if [ "$OS" == "macos" ]; then
        brew install go
    else
        # Linux
        wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
        sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
        rm go1.25.0.linux-amd64.tar.gz
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc 2>/dev/null || true
        export PATH=$PATH:/usr/local/go/bin
    fi
    echo -e "${GREEN}Go 安装完成${NC}"
}

# 安装 Node.js
install_node() {
    if command_exists node; then
        NODE_VERSION=$(node -v)
        echo -e "${GREEN}Node.js 已安装: $NODE_VERSION${NC}"
        return
    fi
    
    echo -e "${YELLOW}正在安装 Node.js 22.x...${NC}"
    if [ "$OS" == "macos" ]; then
        brew install node
    else
        # Linux
        curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
        sudo apt-get install -y nodejs
    fi
    echo -e "${GREEN}Node.js 安装完成${NC}"
}

# 安装 Docker
install_docker() {
    if command_exists docker; then
        DOCKER_VERSION=$(docker -v)
        echo -e "${GREEN}Docker 已安装: $DOCKER_VERSION${NC}"
        return
    fi
    
    echo -e "${YELLOW}正在安装 Docker...${NC}"
    if [ "$OS" == "macos" ]; then
        echo -e "${YELLOW}请手动下载 Docker Desktop for Mac${NC}"
        echo -e "${BLUE}https://www.docker.com/products/docker-desktop${NC}"
        open https://www.docker.com/products/docker-desktop
    else
        # Linux
        curl -fsSL https://get.docker.com | sh
        sudo usermod -aG docker $USER
        sudo systemctl enable docker
        sudo systemctl start docker
    fi
    echo -e "${GREEN}Docker 安装完成${NC}"
}

# 安装 Git
install_git() {
    if command_exists git; then
        echo -e "${GREEN}Git 已安装${NC}"
        return
    fi
    
    echo -e "${YELLOW}正在安装 Git...${NC}"
    if [ "$OS" == "macos" ]; then
        brew install git
    else
        sudo apt-get update && sudo apt-get install -y git
    fi
    echo -e "${GREEN}Git 安装完成${NC}"
}

# 安装项目依赖
install_project_deps() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  正在安装项目依赖...${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    # 后端依赖
    echo -e "${YELLOW}安装 Go 依赖...${NC}"
    go mod download
    
    # 前端依赖
    echo -e "${YELLOW}安装前端依赖...${NC}"
    cd web
    npm install
    cd ..
    
    echo -e "${GREEN}项目依赖安装完成${NC}"
}

# 创建必要目录
create_directories() {
    echo -e "${YELLOW}创建必要目录...${NC}"
    mkdir -p data/news
    mkdir -p data/positions
    mkdir -p decision_logs
    mkdir -p logs
    echo -e "${GREEN}目录创建完成${NC}"
}

# 复制配置文件
setup_config() {
    if [ ! -f config.json ]; then
        echo -e "${YELLOW}复制配置文件模板...${NC}"
        cp config.json.example config.json
        echo -e "${YELLOW}请编辑 config.json 填入你的 API 密钥${NC}"
        echo -e "${YELLOW}运行: python3 scripts/config_editor.py${NC}"
    fi
}

# 主流程
echo ""
echo -e "${BLUE}步骤 1/6: 检查并安装基础工具...${NC}"
if [ "$OS" == "macos" ]; then
    install_homebrew
fi
install_git

echo ""
echo -e "${BLUE}步骤 2/6: 安装 Go...${NC}"
install_go

echo ""
echo -e "${BLUE}步骤 3/6: 安装 Node.js...${NC}"
install_node

echo ""
echo -e "${BLUE}步骤 4/6: 安装 Docker（可选）...${NC}"
install_docker

echo ""
echo -e "${BLUE}步骤 5/6: 安装项目依赖...${NC}"
install_project_deps

echo ""
echo -e "${BLUE}步骤 6/6: 创建目录和配置文件...${NC}"
create_directories
setup_config

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  安装完成！${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}后续步骤:${NC}"
echo -e "  1. 编辑配置: ${YELLOW}python3 scripts/config_editor.py${NC}"
echo -e "  2. 编辑提示词: ${YELLOW}python3 scripts/prompt_editor.py${NC}"
echo -e "  3. 启动后端: ${YELLOW}./main${NC}"
echo -e "  4. 启动前端: ${YELLOW}cd web && npm run dev${NC}"
echo ""
