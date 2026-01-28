#!/usr/bin/env bash
set -euo pipefail

# Configuration
BINARY_NAME="cliproxyapi"
INSTALL_DIR="/opt/homebrew/bin"
SOURCE_DIR="./cmd/server"

echo "Building ${BINARY_NAME}..."

# Get Version Information
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "  Version: ${VERSION}"
echo "  Commit: ${COMMIT}"
echo "  Date:    ${BUILD_DATE}"

# Build with ldflags
go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" \
    -o "${BINARY_NAME}" "${SOURCE_DIR}"

echo "Build complete."

# Check if install directory exists
if [ ! -d "${INSTALL_DIR}" ]; then
    echo "Error: Installation directory ${INSTALL_DIR} does not exist."
    exit 1
fi

# Install
echo "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
if [ -w "${INSTALL_DIR}" ]; then
    mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    echo "Sudo access required to move binary to ${INSTALL_DIR}"
    sudo mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Successfully installed ${BINARY_NAME}"
