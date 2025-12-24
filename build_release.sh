#!/bin/bash

# sing-box 多平台发布构建脚本
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

VERSION=""
SELECTED_PLATFORMS=()

while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--version) VERSION="$2"; shift 2 ;;
        -h|--help) echo "用法: ./build_release.sh -v 版本 [平台...]"; exit 0 ;;
        *) SELECTED_PLATFORMS+=("$1"); shift ;;
    esac
done

[ -z "${VERSION}" ] && VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
VERSION="${VERSION#v}"

TAGS="with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_tailscale"
LDFLAGS="-X 'github.com/sagernet/sing-box/constant.Version=${VERSION}' -s -w -buildid="
OUTPUT_DIR="${SCRIPT_DIR}/release/${VERSION}"
mkdir -p "${OUTPUT_DIR}"

echo -e "${BLUE}�� sing-box 构建 v${VERSION}${NC}"
echo "输出: ${OUTPUT_DIR}"

DEFAULT_PLATFORMS=(
    "linux/amd64" "linux/arm64" "linux/386" "linux/arm/7" "linux/arm/6" "linux/arm/5"
    "linux/mips" "linux/mipsle" "linux/mips64" "linux/mips64le" "linux/riscv64"
    "linux/s390x" "linux/ppc64le" "linux/loong64"
    "windows/amd64" "windows/arm64" "windows/386"
    "darwin/amd64" "darwin/arm64"
    "freebsd/amd64" "freebsd/arm64" "freebsd/386"
)

convert_platform_name() {
    case "$1" in
        linux-amd64) echo "linux/amd64" ;; linux-arm64) echo "linux/arm64" ;;
        linux-386) echo "linux/386" ;; linux-armv7) echo "linux/arm/7" ;;
        linux-armv6) echo "linux/arm/6" ;; linux-armv5) echo "linux/arm/5" ;;
        linux-mips) echo "linux/mips" ;; linux-mipsle) echo "linux/mipsle" ;;
        linux-mips64) echo "linux/mips64" ;; linux-mips64le) echo "linux/mips64le" ;;
        linux-riscv64) echo "linux/riscv64" ;; linux-s390x) echo "linux/s390x" ;;
        linux-ppc64le) echo "linux/ppc64le" ;; linux-loong64) echo "linux/loong64" ;;
        windows-amd64) echo "windows/amd64" ;; windows-arm64) echo "windows/arm64" ;;
        windows-386) echo "windows/386" ;;
        darwin-amd64) echo "darwin/amd64" ;; darwin-arm64) echo "darwin/arm64" ;;
        freebsd-amd64) echo "freebsd/amd64" ;; freebsd-arm64) echo "freebsd/arm64" ;;
        freebsd-386) echo "freebsd/386" ;;
        *) echo "$1" ;;
    esac
}

if [ ${#SELECTED_PLATFORMS[@]} -gt 0 ]; then
    PLATFORMS=()
    for p in "${SELECTED_PLATFORMS[@]}"; do
        PLATFORMS+=("$(convert_platform_name "$p")")
    done
else
    PLATFORMS=("${DEFAULT_PLATFORMS[@]}")
fi

build_platform() {
    local PLATFORM=$1
    local GOOS=$(echo ${PLATFORM} | cut -d'/' -f1)
    local GOARCH=$(echo ${PLATFORM} | cut -d'/' -f2)
    local GOARM=$(echo ${PLATFORM} | cut -d'/' -f3)
    local EXT="" ARCHIVE_EXT="tar.gz"
    
    [ "${GOOS}" = "windows" ] && EXT=".exe" && ARCHIVE_EXT="zip"
    
    local ARCH_NAME="${GOARCH}"
    [ -n "${GOARM}" ] && ARCH_NAME="armv${GOARM}"
    
    local BINARY_NAME="sing-box${EXT}"
    local ARCHIVE_NAME="sing-box-${VERSION}-${GOOS}-${ARCH_NAME}"
    local BUILD_DIR="${OUTPUT_DIR}/${ARCHIVE_NAME}"
    
    echo -e "${BLUE}📦 构建 ${GOOS}/${ARCH_NAME}...${NC}"
    mkdir -p "${BUILD_DIR}"
    
    export GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0
    [ -n "${GOARM}" ] && export GOARM=${GOARM} || unset GOARM
    
    if [ "${GOOS}" = "android" ]; then
        export CGO_ENABLED=1
        if [ -z "${ANDROID_NDK_HOME}" ]; then
            echo -e "${YELLOW}   ⚠️ 跳过: ANDROID_NDK_HOME 未设置${NC}"
            rm -rf "${BUILD_DIR}"
            return 1
        fi
    fi
    
    if go build -trimpath -ldflags "${LDFLAGS}" -tags "${TAGS}" \
        -o "${BUILD_DIR}/${BINARY_NAME}" ./cmd/sing-box; then
        
        if [ ! -f "${BUILD_DIR}/${BINARY_NAME}" ] || [ ! -s "${BUILD_DIR}/${BINARY_NAME}" ]; then
            echo -e "${RED}   ❌ 二进制文件未生成或为空${NC}"
            rm -rf "${BUILD_DIR}"
            return 1
        fi
        
        cp LICENSE README.md "${BUILD_DIR}/" 2>/dev/null || true
        cd "${OUTPUT_DIR}"
        if [ "${ARCHIVE_EXT}" = "zip" ]; then
            zip -r "${ARCHIVE_NAME}.zip" "${ARCHIVE_NAME}" >/dev/null
        else
            tar -czvf "${ARCHIVE_NAME}.tar.gz" "${ARCHIVE_NAME}" >/dev/null
        fi
        rm -rf "${BUILD_DIR}"
        echo -e "${GREEN}   ✅ -> ${ARCHIVE_NAME}.${ARCHIVE_EXT}${NC}"
        cd "${SCRIPT_DIR}"
        return 0
    else
        echo -e "${RED}   ❌ 构建失败${NC}"
        rm -rf "${BUILD_DIR}"
        cd "${SCRIPT_DIR}"
        return 1
    fi
}

echo "构建 ${#PLATFORMS[@]} 个平台..."
SUCCESS_COUNT=0
FAIL_COUNT=0

for PLATFORM in "${PLATFORMS[@]}"; do
    if build_platform "${PLATFORM}"; then
        ((SUCCESS_COUNT++))
    else
        ((FAIL_COUNT++))
    fi
done

cd "${OUTPUT_DIR}"
sha256sum *.tar.gz *.zip 2>/dev/null > checksums.txt || true

echo -e "${GREEN}✅ 完成! 成功: ${SUCCESS_COUNT}, 失败: ${FAIL_COUNT}${NC}"
echo "📁 ${OUTPUT_DIR}"
ls -lh *.tar.gz *.zip 2>/dev/null || true
