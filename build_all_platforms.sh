#!/bin/bash

# 编译所有平台的 sing-box 核心库
# 版本: v1.12.12
# 平台: iOS (arm64 + simulator arm64/x86_64) + macOS (arm64 + x86_64)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="v1.12.12"

echo "=========================================="
echo "编译 sing-box 所有平台核心库"
echo "版本: ${VERSION}"
echo "=========================================="

# 确保 gomobile 在 PATH 中
export PATH="${HOME}/go/bin:${PATH}"

# 检查 gomobile 是否安装
if ! command -v gomobile &> /dev/null; then
    echo "❌ gomobile 未安装，正在安装..."
    go install -v github.com/sagernet/gomobile/cmd/gomobile@v0.1.8
    go install -v github.com/sagernet/gomobile/cmd/gobind@v0.1.8
    gomobile init
fi

cd "${SCRIPT_DIR}"

# 1. 编译 iOS
echo ""
echo "=========================================="
echo "📱 开始编译 iOS..."
echo "=========================================="
./build_all_ios.sh

# 2. 编译 macOS
echo ""
echo "=========================================="
echo "💻 开始编译 macOS..."
echo "=========================================="
./build_all_macos.sh

# 总结
echo ""
echo "=========================================="
echo "✅ 所有平台编译完成！"
echo "=========================================="
echo ""
echo "📦 编译结果："
echo "  - iOS:     ${SCRIPT_DIR}/libbox_output/Libbox.xcframework"
echo "  - macOS:   ${SCRIPT_DIR}/libbox_output_macos/Libbox.xcframework"
echo ""
echo "📋 架构支持："
echo "  iOS:"
echo "    - arm64 (真机)"
echo "    - arm64 (模拟器 - Apple Silicon)"
echo "    - x86_64 (模拟器 - Intel)"
echo ""
echo "  macOS:"
echo "    - arm64 (Apple Silicon)"
echo "    - x86_64 (Intel)"
echo "=========================================="

