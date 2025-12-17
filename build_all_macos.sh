#!/bin/bash

# 编译所有 macOS 架构的 sing-box 核心库
# 版本: v1.12.12

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/libbox_output_macos"
VERSION="v1.12.12"

echo "=========================================="
echo "编译 sing-box macOS 核心库"
echo "版本: ${VERSION}"
echo "输出目录: ${OUTPUT_DIR}"
echo "=========================================="

# 创建输出目录
mkdir -p "${OUTPUT_DIR}"

# 确保 gomobile 在 PATH 中
export PATH="${HOME}/go/bin:${PATH}"

# 检查 gomobile 是否安装
if ! command -v gomobile &> /dev/null; then
    echo "❌ gomobile 未安装，正在安装..."
    go install -v github.com/sagernet/gomobile/cmd/gomobile@v0.1.8
    go install -v github.com/sagernet/gomobile/cmd/gobind@v0.1.8
    gomobile init
fi

# 清理之前的编译文件
echo ""
echo "🧹 清理旧的编译文件..."
rm -rf Libbox.xcframework
rm -rf "${OUTPUT_DIR}/Libbox.xcframework"

# 编译 macOS 库（生产版本，包含所有架构：arm64 + x86_64）
echo ""
echo "📦 开始编译 macOS 库（arm64 + x86_64）..."
cd "${SCRIPT_DIR}"

go run ./cmd/internal/build_libbox -target apple -platform macos

# build_libbox 会将结果复制到 ../sing-box-for-apple/Libbox.xcframework
# 检查编译结果（优先检查 sing-box-for-apple 目录）
FRAMEWORK_SOURCE=""
if [ -d "../sing-box-for-apple/Libbox.xcframework" ]; then
    FRAMEWORK_SOURCE="../sing-box-for-apple/Libbox.xcframework"
    echo ""
    echo "✅ 编译成功！文件在 sing-box-for-apple 目录"
elif [ -d "Libbox.xcframework" ]; then
    FRAMEWORK_SOURCE="Libbox.xcframework"
    echo ""
    echo "✅ 编译成功！文件在当前目录"
else
    echo ""
    echo "❌ 编译失败！Libbox.xcframework 不存在"
    exit 1
fi

# 显示架构信息
echo ""
echo "📋 编译的架构："
find "${FRAMEWORK_SOURCE}" -name "Libbox" -type f | while read lib; do
    echo "  - $(file "$lib" | cut -d: -f2)"
done

# 复制到输出目录
echo ""
echo "💾 保存到输出目录: ${OUTPUT_DIR}..."
rm -rf "${OUTPUT_DIR}/Libbox.xcframework"
cp -R "${FRAMEWORK_SOURCE}" "${OUTPUT_DIR}/"

# 创建版本信息文件
echo "sing-box version: ${VERSION}" > "${OUTPUT_DIR}/VERSION.txt"
echo "build date: $(date)" >> "${OUTPUT_DIR}/VERSION.txt"
echo "build command: go run ./cmd/internal/build_libbox -target apple -platform macos" >> "${OUTPUT_DIR}/VERSION.txt"
echo "platform: macOS" >> "${OUTPUT_DIR}/VERSION.txt"
echo "architectures: arm64, x86_64" >> "${OUTPUT_DIR}/VERSION.txt"

# 显示文件大小
echo ""
echo "📊 文件大小："
du -sh "${OUTPUT_DIR}/Libbox.xcframework"

echo ""
echo "=========================================="
echo "✅ 编译完成！"
echo "输出位置: ${OUTPUT_DIR}/Libbox.xcframework"
echo "=========================================="

