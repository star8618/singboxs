#!/bin/bash

# sing-box 官方源码同步脚本
# 自动同步官方最新代码，同时保留你的魔改功能
#
# 使用方法:
#   ./sync_upstream.sh              # 同步到官方最新稳定版
#   ./sync_upstream.sh v1.12.13     # 同步到指定版本
#
# 工作原理:
#   1. 备份当前状态到分支
#   2. 生成魔改代码的 patch
#   3. 重置到官方最新版本
#   4. 重新应用魔改代码

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 官方仓库地址
UPSTREAM_URL="https://github.com/SagerNet/sing-box.git"
UPSTREAM_NAME="upstream"

# 你的魔改文件列表（新增的文件，需要单独处理）
MY_NEW_FILES=(
    "common/tracker/"
    "experimental/libbox/debug_server.go"
    "protocol/group/failover.go"
    "libbox_output/"
)

echo -e "${BLUE}"
echo "╔════════════════════════════════════════╗"
echo "║   🔄 Sing-box 官方源码同步脚本         ║"
echo "╚════════════════════════════════════════╝"
echo -e "${NC}"

# 检查是否有未提交的更改
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo -e "${YELLOW}⚠️  检测到未提交的更改，请先提交或暂存${NC}"
    git status --short
    exit 1
fi

# 检查并添加 upstream 远程仓库
if ! git remote | grep -q "^${UPSTREAM_NAME}$"; then
    echo -e "${BLUE}📌 添加官方远程仓库...${NC}"
    git remote add ${UPSTREAM_NAME} ${UPSTREAM_URL}
fi

# 获取官方最新代码
echo -e "${BLUE}📥 获取官方最新代码...${NC}"
if ! git fetch ${UPSTREAM_NAME} --tags --force 2>&1; then
    echo -e "${RED}❌ 获取失败，请检查网络连接${NC}"
    echo "   提示: 可能需要开启代理"
    exit 1
fi

# 获取官方最新稳定版
LATEST_STABLE=$(git tag -l "v1.*" --sort=-version:refname | grep -v "alpha\|beta\|rc" | head -1)
echo -e "${GREEN}📌 官方最新稳定版: ${LATEST_STABLE}${NC}"

# 确定目标版本
if [ -n "$1" ]; then
    TARGET="$1"
    if [[ "$TARGET" =~ ^[0-9] ]]; then
        TARGET="v${TARGET}"
    fi
else
    TARGET="${LATEST_STABLE}"
fi

echo -e "${YELLOW}🎯 目标版本: ${TARGET}${NC}"

# 检查目标是否存在
if ! git rev-parse "${TARGET}" &>/dev/null; then
    echo -e "${RED}❌ 目标版本 ${TARGET} 不存在${NC}"
    echo "   可用的稳定版本:"
    git tag -l "v1.*" --sort=-version:refname | grep -v "alpha\|beta\|rc" | head -10
    exit 1
fi

# 获取当前基于的官方版本（从最近的官方 tag 推断）
CURRENT_BASE=$(git describe --tags --abbrev=0 --match "v*" 2>/dev/null || echo "unknown")
echo -e "${GREEN}📌 当前基于版本: ${CURRENT_BASE}${NC}"

# 检查是否已经是最新
if [ "${CURRENT_BASE}" = "${TARGET}" ]; then
    echo -e "${GREEN}✅ 已经是最新版本 ${TARGET}${NC}"
    exit 0
fi

echo ""
echo -e "${BLUE}📦 开始同步: ${CURRENT_BASE} → ${TARGET}${NC}"
echo ""

# 创建备份分支
BACKUP_BRANCH="backup-$(date +%Y%m%d_%H%M%S)"
echo -e "${BLUE}1️⃣  创建备份分支: ${BACKUP_BRANCH}${NC}"
git branch -D ${BACKUP_BRANCH} 2>/dev/null || true
git branch ${BACKUP_BRANCH}

# 生成魔改代码的 patch
echo -e "${BLUE}2️⃣  保存魔改代码为 patch...${NC}"
PATCH_FILE="/tmp/singbox_mods_$(date +%Y%m%d_%H%M%S).patch"
git diff ${CURRENT_BASE} HEAD -- . ':!libbox_output' > "${PATCH_FILE}" 2>/dev/null || true
echo "   Patch 文件: ${PATCH_FILE}"

# 重置到官方目标版本
echo -e "${BLUE}3️⃣  重置到官方 ${TARGET}...${NC}"
git reset --hard ${TARGET}

# 应用魔改代码
echo -e "${BLUE}4️⃣  应用魔改代码...${NC}"

# 先应用 patch（修改过的官方文件）
if [ -s "${PATCH_FILE}" ]; then
    if git apply "${PATCH_FILE}" --3way 2>&1 | grep -v "^Falling back" | grep -v "trailing whitespace"; then
        echo -e "${GREEN}   ✓ Patch 应用成功${NC}"
    else
        echo -e "${YELLOW}   ⚠ 部分 patch 可能有冲突，请检查${NC}"
    fi
fi

# 恢复新增的魔改文件
echo -e "${BLUE}5️⃣  恢复新增的魔改文件...${NC}"
for item in "${MY_NEW_FILES[@]}"; do
    if git ls-tree -r ${BACKUP_BRANCH} --name-only | grep -q "^${item}"; then
        git checkout ${BACKUP_BRANCH} -- "${item}" 2>/dev/null && echo "   ✓ ${item}"
    fi
done

# 提交更改
echo -e "${BLUE}6️⃣  提交更改...${NC}"
git add -A
git commit -m "升级到官方 ${TARGET} + 保留魔改功能" 2>/dev/null || echo "   (无新更改需要提交)"

echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}✅ 同步完成！${NC}"
echo -e "${GREEN}   版本: ${CURRENT_BASE} → ${TARGET}${NC}"
echo -e "${GREEN}   备份分支: ${BACKUP_BRANCH}${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo "后续操作:"
echo "  1. 测试构建: ./build_release.sh ${TARGET#v}"
echo "  2. 推送到 GitHub: git push origin main --force"
echo "  3. 如需回滚: git reset --hard ${BACKUP_BRANCH}"
