#!/usr/bin/env bash
set -e

# ===== TeChat One-Line Installer =====
# Usage: bash <(curl -sf https://raw.githubusercontent.com/Krembovan/techat/main/install.sh)

BOLD='\033[1m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; NC='\033[0m'

echo -e "${BOLD}${CYAN}  TeChat Installer${NC}"
echo "================================"

# ---- Detect system ----
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; esac
case "$OS" in linux|darwin) ;; *) echo "Unsupported: $OS $ARCH"; exit 1 ;; esac

# ---- Install Go if missing ----
if ! command -v go &>/dev/null; then
	echo -e "${YELLOW}Go not found. Downloading Go 1.22.3 for ${OS}/${ARCH}...${NC}"
	curl -sfL "https://go.dev/dl/go1.22.3.${OS}-${ARCH}.tar.gz" | tar -C /tmp -xz
	export PATH="/tmp/go/bin:$PATH"
	export GOROOT="/tmp/go"
	echo -e "${GREEN}Go installed from /tmp/go${NC}"
fi

# ---- Build techat ----
echo -e "${CYAN}Building TeChat...${NC}"
TMPDIR=$(mktemp -d)
curl -sfL "https://github.com/Krembovan/techat/archive/refs/heads/main.tar.gz" | tar -xz -C "$TMPDIR"
cd "$TMPDIR/techat-main"

go mod tidy
go build -ldflags="-s -w" -o techat ./cmd/techat

# ---- Install to ~/.local/bin ----
mkdir -p "$HOME/.local/bin"
cp techat "$HOME/.local/bin/techat"
chmod +x "$HOME/.local/bin/techat"
export PATH="$HOME/.local/bin:$PATH"

# Add to shell profile if not already there
if ! grep -q '\.local/bin' "$HOME/.bashrc" 2>/dev/null; then
	echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"
fi
if [ -f "$HOME/.zshrc" ] && ! grep -q '\.local/bin' "$HOME/.zshrc" 2>/dev/null; then
	echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.zshrc"
fi

# ---- Cleanup ----
rm -rf "$TMPDIR"

# ---- Start! ----
echo ""
echo -e "${GREEN}${BOLD}✓ TeChat installed!${NC}"
echo -e "${CYAN}  Starting TeChat...  (relay not needed for first look)${NC}"
echo ""

techat --username "$(whoami)"
