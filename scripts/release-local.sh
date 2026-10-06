#!/usr/bin/env bash
# 本地发布：把当前工作区构建成可执行包，**覆盖式解压**到 release/SingleSourceReady/。
#
#   ./scripts/release-local.sh              # 构建 + 覆盖 release/SingleSourceReady/
#   make local                              # 同上（Makefile 里的固定入口）
#
# 与正式发布（`go run ./cmd/ssr-core release`）的区别：
#   - 允许脏工作区（版本带 +dirty），给本地自测用；
#   - 产物固定是 release/SingleSourceReady/，**文件名里不带版本号**，每次直接覆盖上一次的；
#   - 用「解压覆盖」的方式更新：只替换包里带的文件（SingleSourceReady.app /
#     config/defaults / data/template / resource / VERSION），
#     用户改过的 config/*.yaml、config/.backup/、data/udi_data.sqlite3*、output/ 一律不动。
#
# 版本号仍然写在 VERSION 与「关于」窗口里（来自 git tag + 工作区是否干净）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="$ROOT/release/SingleSourceReady"
OUT_REL="build/local-publish"   # --out 走相对路径（相对项目根），临时产物目录
SCRATCH="$ROOT/$OUT_REL"

cd "$ROOT"
rm -rf "$SCRATCH"

echo "▸ 构建（wails build + 组装发布包；允许脏工作区）"
go run ./cmd/ssr-core release --allow-dirty --out "$OUT_REL"

ARCHIVE="$(ls "$SCRATCH"/*.zip 2>/dev/null | head -1)"
if [ -z "$ARCHIVE" ]; then
  echo "没有生成压缩包：$SCRATCH" >&2
  exit 1
fi

DATABASE="$TARGET/data/udi_data.sqlite3"
BEFORE="（还没有数据库）"
if [ -f "$DATABASE" ]; then
  BEFORE="$(du -h "$DATABASE" | cut -f1)"
fi

mkdir -p "$TARGET"
echo "▸ 解压覆盖到 $TARGET"
unzip -oq "$ARCHIVE" -d "$TARGET"

# 临时目录清掉：release/ 下只留固定名字的那一份
rm -rf "$SCRATCH"

AFTER="（还没有数据库）"
if [ -f "$DATABASE" ]; then
  AFTER="$(du -h "$DATABASE" | cut -f1)"
fi

echo
echo "▸ 已更新：$TARGET"
echo "  版本　　：$(tr '\n' ' ' < "$TARGET/VERSION")"
echo "  数据库　：${BEFORE} → ${AFTER}（未覆盖）"
echo "  保留未动：config/*.yaml、config/.backup/、data/udi_data.sqlite3*、output/"
