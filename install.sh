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

# First try /releases/latest, then fallback to /releases (which includes pre-releases)
LATEST_RELEASE=$(curl -s https://api.github.com/repos/$REPO/releases/latest)
if echo "$LATEST_RELEASE" | grep -q "Not Found"; then
    LATEST_RELEASE=$(curl -s https://api.github.com/repos/$REPO/releases | grep -v '\[\]' | head -n 50 | tr -d '\n' | sed 's/},{/}\n{/g' | head -n 1)
fi

TAG=$(echo "$LATEST_RELEASE" | grep '"tag_name":' | head -n 1 | cut -d'"' -f4 || echo "")

if [[ -n "$TAG" ]]; then
    echo -e "${GREEN}Found version: $TAG${NC}"
    
    # Construct the asset name pattern
    ASSET_PATTERN="motoko_${OS}_${ARCH}"
    DOWNLOAD_URL=$(echo "$LATEST_RELEASE" | grep "browser_download_url" | grep "$ASSET_PATTERN" | head -n 1 | cut -d'"' -f4 || echo "")

    if [[ -n "$DOWNLOAD_URL" ]]; then
        echo -e "${BLUE}Downloading $DOWNLOAD_URL...${NC}"
        TEMP_FILE=$(mktemp)
        curl -sSL "$DOWNLOAD_URL" -o "$TEMP_FILE"
        
        mkdir -p "$INSTALL_DIR"
        
        # If it's a tarball, extract it. If it's a raw binary, just move it.
        if [[ "$DOWNLOAD_URL" == *.tar.gz ]]; then
            EXTRACT_DIR=$(mktemp -d)
            tar -xzf "$TEMP_FILE" -C "$EXTRACT_DIR"
            
            # Find the binary in the extracted archive
            if [[ -f "$EXTRACT_DIR/$BINARY_NAME" ]]; then
                mv "$EXTRACT_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
            elif [[ -f "$EXTRACT_DIR/motoko_${OS}_${ARCH}" ]]; then
                mv "$EXTRACT_DIR/motoko_${OS}_${ARCH}" "$INSTALL_DIR/$BINARY_NAME"
            else
                # Fallback: search for any file containing the binary name, or any file that isn't doc/license
                FOUND_BIN=$(find "$EXTRACT_DIR" -type f -name "*motoko*" | head -n 1)
                if [[ -z "$FOUND_BIN" ]]; then
                    FOUND_BIN=$(find "$EXTRACT_DIR" -type f ! -name "*.md" ! -name "LICENSE*" ! -name "COPYING*" | head -n 1)
                fi
                
                if [[ -n "$FOUND_BIN" ]]; then
                    mv "$FOUND_BIN" "$INSTALL_DIR/$BINARY_NAME"
                else
                    echo -e "${RED}Error: Extract failed, could not locate binary in archive.${NC}"
                    rm -rf "$EXTRACT_DIR"
                    exit 1
                fi
            fi
            chmod +x "$INSTALL_DIR/$BINARY_NAME"
            rm -rf "$EXTRACT_DIR"
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
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo -e "${YELLOW}Warning: $INSTALL_DIR is not in your \$PATH.${NC}"
    if [[ "$OS" == "darwin" ]]; then
        echo -e "To add it to your path, run:"
        echo -e "  ${GREEN}echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc && source ~/.zshrc${NC}"
    else
        echo -e "To add it to your path, add the following to your shell profile (e.g. ~/.bashrc or ~/.profile):"
        echo -e "  ${GREEN}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
    fi
else
    echo -e "Run ${GREEN}motoko${NC} to start."
fi
