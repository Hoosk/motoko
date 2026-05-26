#!/bin/bash

# Motoko - Smart Installation Script
# This script detects your OS/Arch and downloads the latest pre-built binary
# from GitHub Releases. If no binary is found, it falls back to building from source.

set -e

REPO="Hoosk/motoko"
INSTALL_DIR="$HOME/.local/bin"
BINARY_NAME="motoko"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}Starting Motoko installation...${NC}"

# 1. Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

# We only support Linux and macOS (Darwin)
if [[ "$OS" != "linux" && "$OS" != "darwin" ]]; then
    echo -e "${RED}Unsupported OS: $OS${NC}"
    exit 1
fi

# 2. Try to fetch the latest release from GitHub API
echo -e "${BLUE}Checking for pre-built binaries for ${OS}/${ARCH}...${NC}"
LATEST_RELEASE=$(curl -s https://api.github.com/repos/$REPO/releases/latest)
TAG=$(echo "$LATEST_RELEASE" | grep -m1 '"tag_name":' | cut -d'"' -f4)

if [[ -n "$TAG" ]]; then
    echo -e "${GREEN}Found latest version: $TAG${NC}"
    
    # Construct the asset name (assuming standard naming: motoko_Linux_x86_64.tar.gz etc.)
    # Note: Adjust naming convention if your GitHub Action uses a different one
    ASSET_PATTERN="motoko_${OS}_${ARCH}"
    DOWNLOAD_URL=$(echo "$LATEST_RELEASE" | grep "browser_download_url" | grep "$ASSET_PATTERN" | cut -d'"' -f4 | head -n 1)

    if [[ -n "$DOWNLOAD_URL" ]]; then
        echo -e "${BLUE}Downloading $DOWNLOAD_URL...${NC}"
        TEMP_FILE=$(mktemp)
        curl -sSL "$DOWNLOAD_URL" -o "$TEMP_FILE"
        
        mkdir -p "$INSTALL_DIR"
        
        # If it's a tarball, extract it. If it's a raw binary, just move it.
        if [[ "$DOWNLOAD_URL" == *.tar.gz ]]; then
            tar -xzf "$TEMP_FILE" -C "$INSTALL_DIR" "$BINARY_NAME"
        else
            mv "$TEMP_FILE" "$INSTALL_DIR/$BINARY_NAME"
            chmod +x "$INSTALL_DIR/$BINARY_NAME"
        fi
        
        rm -f "$TEMP_FILE"
        echo -e "${GREEN}Successfully installed Motoko pre-built binary!${NC}"
    else
        echo -e "${YELLOW}No pre-built binary found for your platform. Falling back to source build...${NC}"
        FALLBACK_BUILD=true
    fi
else
    echo -e "${YELLOW}No GitHub releases found. Falling back to source build...${NC}"
    FALLBACK_BUILD=true
fi

# 3. Fallback: Build from source
if [[ "$FALLBACK_BUILD" == true ]]; then
    if ! command -v go &> /dev/null; then
        echo -e "${RED}Error: Go is not installed. Source build requires Go 1.24+.${NC}"
        exit 1
    fi

    echo -e "${BLUE}Building from source...${NC}"
    TEMP_DIR=$(mktemp -d)
    git clone https://github.com/$REPO.git "$TEMP_DIR"
    cd "$TEMP_DIR"
    go build -o "$BINARY_NAME" ./cmd/motoko
    
    mkdir -p "$INSTALL_DIR"
    mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    echo -e "${GREEN}Successfully built and installed Motoko from source!${NC}"
fi

echo -e ""
echo -e "Make sure ${BLUE}$INSTALL_DIR${NC} is in your ${BLUE}\$PATH${NC}."
echo -e "Run ${GREEN}motoko${NC} to start."
