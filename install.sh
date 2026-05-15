#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/techat/techat"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BOLD}${CYAN}TeChat Installer${NC}"
echo "================================"

# Detect OS/arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        echo "Building from source instead..."
        BUILD_FROM_SOURCE=true
        ;;
esac

case "$OS" in
    linux|darwin) ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        echo "Building from source instead..."
        BUILD_FROM_SOURCE=true
        ;;
esac

if command -v go &> /dev/null && [ "${BUILD_FROM_SOURCE:-false}" = "true" ]; then
    echo -e "${CYAN}Cloning repository...${NC}"
    git clone --depth 1 "$REPO.git" "$TMP_DIR/techat" 2>/dev/null || {
        echo -e "${RED}Failed to clone. Do you have git installed?${NC}"
        exit 1
    }
    cd "$TMP_DIR/techat"
    echo -e "${CYAN}Building TeChat...${NC}"
    go build -trimpath -ldflags="-s -w" -o techat ./cmd/techat
    go build -trimpath -ldflags="-s -w" -o techat-relay ./cmd/relay
    echo -e "${CYAN}Installing...${NC}"
    install -m 755 techat "$INSTALL_DIR/techat"
    install -m 755 techat-relay "$INSTALL_DIR/techat-relay"
elif command -v go &> /dev/null; then
    echo -e "${CYAN}Downloading and building...${NC}"
    go install "${REPO}/cmd/techat@latest" 2>/dev/null && \
        echo -e "${GREEN}Installed via 'go install'${NC}" && \
        echo "Make sure \$GOPATH/bin is in your PATH" && \
        exit 0 || {
        echo -e "${RED}Go install failed, building from source...${NC}"
        git clone --depth 1 "$REPO.git" "$TMP_DIR/techat" 2>/dev/null
        cd "$TMP_DIR/techat"
        go build -trimpath -ldflags="-s -w" -o techat ./cmd/techat
        go build -trimpath -ldflags="-s -w" -o techat-relay ./cmd/relay
        install -m 755 techat "$INSTALL_DIR/techat"
        install -m 755 techat-relay "$INSTALL_DIR/techat-relay"
    }
else
    # Download pre-built binary (future use)
    DOWNLOAD_URL="${REPO}/releases/latest/download/techat-${OS}-${ARCH}"
    echo -e "${CYAN}Downloading TeChat for ${OS}/${ARCH}...${NC}"
    if curl -sfL "$DOWNLOAD_URL" -o "$TMP_DIR/techat" 2>/dev/null; then
        chmod +x "$TMP_DIR/techat"
        install -m 755 "$TMP_DIR/techat" "$INSTALL_DIR/techat"
        echo -e "${GREEN}Downloaded pre-built binary${NC}"
    else
        echo -e "${RED}No pre-built binary available for ${OS}/${ARCH}${NC}"
        echo -e "${RED}Install Go from https://go.dev/dl/ and try again${NC}"
        echo -e "${RED}Or use: go install ${REPO}/cmd/techat@latest${NC}"
        exit 1
    fi
fi

echo ""
echo -e "${GREEN}${BOLD}✓ TeChat installed successfully!${NC}"
echo ""
echo "  Start the relay server (first terminal):"
echo -e "    ${CYAN}techat-relay${NC}"
echo ""
echo "  Connect with your friends:"
echo -e "    ${CYAN}techat --relay <relay-ip>:7777 --username <name>${NC}"
echo ""
echo "  Quick start (local mode):"
echo -e "    ${CYAN}techat${NC}"
echo ""
echo "  Panic button: ${BOLD}Ctrl+Esc${NC} (wipes all local data)"
echo "  Help: ${BOLD}/help${NC} in the app"
echo ""
echo -e "${BOLD}Make sure port 7777 is open for the relay server!${NC}"
