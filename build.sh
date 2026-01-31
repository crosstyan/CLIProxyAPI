#!/bin/bash
#
# build.sh - Universal Build Script for CLIProxyAPI
#
# This script builds CLIProxyAPI for multiple platforms with configurable
# default config paths. It supports:
#   - Native macOS builds (ARM64 and AMD64)
#   - Native Linux builds
#   - Cross-compilation from macOS to Linux x86_64
#
# Usage:
#   ./build.sh                          # Build for current platform
#   ./build.sh --homebrew               # Build for Homebrew with /opt/homebrew/etc config path
#   ./build.sh --target linux-amd64     # Cross-compile for Linux x86_64
#   ./build.sh --target darwin-arm64    # Build for macOS Apple Silicon
#   ./build.sh --target darwin-amd64    # Build for macOS Intel
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_NAME="cli-proxy-api-plus"
MAIN_PACKAGE="./cmd/server/"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo 'none')}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

DEFAULT_CONFIG_PATH="${DEFAULT_CONFIG_PATH:-}"
TARGET="${TARGET:-native}"

while [[ $# -gt 0 ]]; do
    case $1 in
        --homebrew)
            if [[ "$(uname -m)" == "arm64" ]]; then
                DEFAULT_CONFIG_PATH="/opt/homebrew/etc/cliproxyapi.conf"
            else
                DEFAULT_CONFIG_PATH="/usr/local/etc/cliproxyapi.conf"
            fi
            shift
            ;;
        --target)
            TARGET="$2"
            shift 2
            ;;
        --config-path)
            DEFAULT_CONFIG_PATH="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        -h|--help)
            cat << 'EOF'
Usage: ./build.sh [OPTIONS]

Build CLIProxyAPI for various platforms.

OPTIONS:
    --homebrew              Build for Homebrew installation (sets appropriate
                            default config path: /opt/homebrew/etc/cliproxyapi.conf
                            on Apple Silicon, /usr/local/etc/cliproxyapi.conf on Intel)
    --target PLATFORM       Target platform (default: native)
                            Supported: darwin-arm64, darwin-amd64, linux-arm64, linux-amd64
    --config-path PATH      Set default configuration file path at compile time
                            (only applies to macOS builds)
    --version VERSION       Override version string
    -h, --help              Show this help message

EXAMPLES:
    ./build.sh                          # Build for current platform
    ./build.sh --homebrew               # Build for Homebrew
    ./build.sh --target linux-amd64     # Cross-compile for Linux x86_64
    ./build.sh --config-path /etc/cliproxyapi/config.yaml

ENVIRONMENT VARIABLES:
    VERSION             Version string (default: git describe output)
    COMMIT              Git commit hash (default: git rev-parse output)
    BUILD_DATE          Build timestamp (default: current UTC time)
    DEFAULT_CONFIG_PATH Default config path compiled into binary
    CGO_ENABLED         Set to 1 to enable CGO (default: 0)
    OUTPUT_DIR          Output directory for binaries (default: ./dist)
EOF
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            echo "Use -h or --help for usage information" >&2
            exit 1
            ;;
    esac
done

OUTPUT_DIR="${OUTPUT_DIR:-${SCRIPT_DIR}/dist}"
mkdir -p "${OUTPUT_DIR}"

if [[ "${TARGET}" == "native" ]]; then
    GOOS="$(go env GOOS)"
    GOARCH="$(go env GOARCH)"
else
    GOOS="${TARGET%%-*}"
    GOARCH="${TARGET##*-}"
fi

case "${GOARCH}" in
    amd64|x86_64) GOARCH="amd64" ;;
    arm64|aarch64) GOARCH="arm64" ;;
esac

case "${GOOS}" in
    darwin|linux|windows) ;;
    *) echo "Error: Unsupported OS: ${GOOS}" >&2; exit 1 ;;
esac

case "${GOARCH}" in
    amd64|arm64) ;;
    *) echo "Error: Unsupported architecture: ${GOARCH}" >&2; exit 1 ;;
esac

if [[ "${GOOS}" == "linux" && "$(uname -s)" == "Darwin" && "${CGO_ENABLED:-0}" == "1" ]]; then
    echo "Warning: Cross-compiling with CGO from macOS to Linux requires a cross-compiler." >&2
    echo "Set CGO_ENABLED=0 to build without CGO." >&2
    exit 1
fi

OUTPUT_PATH="${OUTPUT_DIR}/${GOOS}-${GOARCH}/cliproxyapi"
[[ "${GOOS}" == "windows" ]] && OUTPUT_PATH="${OUTPUT_PATH}.exe"

LDFLAGS="-s -w"
LDFLAGS="${LDFLAGS} -X 'main.Version=${VERSION}'"
LDFLAGS="${LDFLAGS} -X 'main.Commit=${COMMIT}'"
LDFLAGS="${LDFLAGS} -X 'main.BuildDate=${BUILD_DATE}'"

if [[ -n "${DEFAULT_CONFIG_PATH}" && "${GOOS}" == "darwin" ]]; then
    LDFLAGS="${LDFLAGS} -X 'main.DefaultConfigPath=${DEFAULT_CONFIG_PATH}'"
fi

export GOOS="${GOOS}"
export GOARCH="${GOARCH}"
export CGO_ENABLED="${CGO_ENABLED:-0}"

echo "========================================"
echo "Building ${PROJECT_NAME}"
echo "========================================"
echo "Version:        ${VERSION}"
echo "Commit:         ${COMMIT}"
echo "Build Date:     ${BUILD_DATE}"
echo "Target OS:      ${GOOS}"
echo "Target Arch:    ${GOARCH}"
echo "CGO Enabled:    ${CGO_ENABLED}"
[[ -n "${DEFAULT_CONFIG_PATH}" && "${GOOS}" == "darwin" ]] && echo "Config Path:    ${DEFAULT_CONFIG_PATH}"
echo "Output:         ${OUTPUT_PATH}"
echo "========================================"

go build -ldflags "${LDFLAGS}" -o "${OUTPUT_PATH}" "${MAIN_PACKAGE}"

if [[ -f "${OUTPUT_PATH}" ]]; then
    echo "Build successful!"
    ls -lh "${OUTPUT_PATH}"
    
    if [[ "${TARGET}" == "native" ]] || [[ "${GOOS}" == "$(go env GOOS)" && "${GOARCH}" == "$(go env GOARCH)" ]]; then
        echo ""
        "${OUTPUT_PATH}" --help 2>&1 | head -1 || true
    fi
    
    if [[ "${GOOS}" == "linux" && "$(uname -s)" == "Darwin" ]]; then
        FILE_INFO=$(file "${OUTPUT_PATH}")
        echo ""
        echo "File info: ${FILE_INFO}"
    fi
else
    echo "Error: Build failed - output file not found" >&2
    exit 1
fi

mkdir -p "$(dirname "${OUTPUT_PATH}")"

echo ""
echo "Build complete: ${OUTPUT_PATH}"
