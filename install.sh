#!/bin/bash
# =============================================================================
# AI新闻量化交易系统 - 一键环境安装脚本
# 支持 macOS / Linux (Ubuntu/Debian/CentOS/Alpine)
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
PACKAGE_MANAGER=""

if [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macos"
    echo -e "${GREEN}检测到操作系统: macOS${NC}"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="linux"
    echo -e "${GREEN}检测到操作系统: Linux${NC}"
    
    # 检测Linux发行版和包管理器
    if command -v apt-get &> /dev/null; then
        PACKAGE_MANAGER="apt"
        echo -e "${GREEN}包管理器: apt-get (Ubuntu/Debian)${NC}"
    elif command -v yum &> /dev/null; then
        PACKAGE_MANAGER="yum"
        echo -e "${GREEN}包管理器: yum (CentOS/RHEL)${NC}"
    elif command -v apk &> /dev/null; then
        PACKAGE_MANAGER="apk"
        echo -e "${GREEN}包管理器: apk (Alpine)${NC}"
    else
        echo -e "${YELLOW}警告: 未检测到包管理器${NC}"
    fi
else
    echo -e "${RED}不支持的操作系统: $OSTYPE${NC}"
    exit 1
fi

# 检查命令是否存在
command_exists() {
    command -v "$1" &> /dev/null
}

# 安装系统包（自动检测包管理器）
install_system_package() {
    local pkg_name=$1
    
    if command_exists "$pkg_name"; then
        return 0
    fi
    
    echo -e "${YELLOW}正在安装 $pkg_name...${NC}"
    
    case $PACKAGE_MANAGER in
        apt)
            apt-get update -qq && apt-get install -y -qq "$pkg_name"
            ;;
        yum)
            yum install -y "$pkg_name"
            ;;
        apk)
            apk add --no-cache "$pkg_name"
            ;;
        *)
            echo -e "${RED}错误: 无法安装 $pkg_name，请手动安装${NC}"
            return 1
            ;;
    esac
    
    echo -e "${GREEN}$pkg_name 安装完成${NC}"
}

# 通用下载函数（curl/wget自动选择）
download_file() {
    local url=$1
    local output=$2
    
    if command_exists curl; then
        curl -fsSL "$url" -o "$output"
    elif command_exists wget; then
        wget -q "$url" -O "$output"
    else
        echo -e "${YELLOW}安装下载工具...${NC}"
        install_system_package curl || install_system_package wget
        
        # 重试下载
        if command_exists curl; then
            curl -fsSL "$url" -o "$output"
        elif command_exists wget; then
            wget -q "$url" -O "$output"
        else
            echo -e "${RED}错误: 无法安装下载工具${NC}"
            return 1
        fi
    fi
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
        # Linux - 使用通用下载函数
        local go_version="1.25.0"
        local go_tar="go${go_version}.linux-amd64.tar.gz"
        
        echo -e "${YELLOW}下载 Go ${go_version}...${NC}"
        download_file "https://go.dev/dl/${go_tar}" "${go_tar}"
        
        echo -e "${YELLOW}解压安装...${NC}"
        tar -C /usr/local -xzf "${go_tar}"
        rm -f "${go_tar}"
        
        # 添加到PATH
        if ! grep -q "/usr/local/go/bin" ~/.bashrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        fi
        if [ -f ~/.zshrc ] && ! grep -q "/usr/local/go/bin" ~/.zshrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
        fi
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
        case $PACKAGE_MANAGER in
            apt)
                curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
                apt-get install -y nodejs
                ;;
            yum)
                curl -fsSL https://rpm.nodesource.com/setup_22.x | bash -
                yum install -y nodejs
                ;;
            apk)
                apk add --no-cache nodejs npm
                ;;
            *)
                echo -e "${RED}错误: 无法自动安装 Node.js${NC}"
                echo -e "${YELLOW}请手动安装 Node.js 22+${NC}"
                return 1
                ;;
        esac
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
    else
        # Linux
        download_file "https://get.docker.com" "get-docker.sh"
        sh get-docker.sh
        rm -f get-docker.sh
        
        if command_exists systemctl; then
            systemctl enable docker
            systemctl start docker
        fi
    fi
    
    echo -e "${GREEN}Docker 安装完成${NC}"
}

# 安装 Git
install_git() {
    if command_exists git; then
        echo -e "${GREEN}Git 已安装: $(git --version)${NC}"
        return
    fi
    
    echo -e "${YELLOW}正在安装 Git...${NC}"
    
    if [ "$OS" == "macos" ]; then
        brew install git
    else
        install_system_package git
    fi
    
    echo -e "${GREEN}Git 安装完成${NC}"
}

# 安装项目依赖
install_project_deps() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  正在安装项目依赖...${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    # 确保Go可用
    if ! command_exists go; then
        export PATH=$PATH:/usr/local/go/bin
    fi
    
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
        echo -e "${YELLOW}运行: python3 start.py${NC}"
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
echo -e "  1. 编辑配置: ${YELLOW}python3 start.py${NC}"
echo -e "  2. 启动后端: ${YELLOW}go run main.go${NC} 或 ${YELLOW}./main${NC}"
echo -e "  3. 启动前端: ${YELLOW}cd web && npm run dev${NC}"
echo ""
