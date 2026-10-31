#!/usr/bin/env bash
# 建一个"假共享盘"骨架，供本机演练使用（P0）。
#
# 目录结构与 docs/design/multiuser-workflow.md §4 一致：
#   published/{ra,eama,udi}/  state/state.json  consolidated/  audit/
#
# 用法：
#   ./scripts/ssr-share-init.sh --root /tmp/ssr-share
#   ./scripts/ssr-share-init.sh --root /tmp/ssr-share --force   # 目录非空也继续（不删已有文件）
set -euo pipefail

usage() {
	cat <<'USAGE'
用法: ssr-share-init.sh --root <绝对路径> [--force]

  --root   共享盘根目录（必须绝对路径；本地目录用来模拟共享盘）
  --force  根目录已存在且非空时仍然写入（不会删除已有文件）
  -h       显示本帮助
USAGE
}

ROOT=""
FORCE=0
while [[ $# -gt 0 ]]; do
	case "$1" in
	--root)
		ROOT="${2:-}"
		shift 2
		;;
	--force)
		FORCE=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "未知参数: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [[ -z "$ROOT" ]]; then
	echo "--root 是必填项" >&2
	usage >&2
	exit 1
fi
if [[ "$ROOT" != /* ]]; then
	echo "--root 必须是绝对路径（当前: $ROOT）" >&2
	exit 1
fi
if [[ -e "$ROOT" && ! -d "$ROOT" ]]; then
	echo "已存在同名文件，不是目录: $ROOT" >&2
	exit 1
fi
if [[ -d "$ROOT" && -n "$(ls -A "$ROOT" 2>/dev/null || true)" && "$FORCE" -eq 0 ]]; then
	echo "目录已存在且非空: $ROOT（要沿用请加 --force）" >&2
	exit 1
fi

mkdir -p "$ROOT/published/ra" "$ROOT/published/eama" "$ROOT/published/udi" \
	"$ROOT/state" "$ROOT/consolidated" "$ROOT/audit"

STATE="$ROOT/state/state.json"
if [[ ! -f "$STATE" ]]; then
	# 原子写：先落临时文件再 rename（与程序的发布/状态写入同一口径）
	cat >"$STATE.tmp" <<'JSON'
{
  "generation": 0,
  "sources": {
    "ra_input": null,
    "product_category": null,
    "medical_insurance_code": null,
    "global_udi_input": null
  },
  "last_consolidation": null,
  "consolidation_running": null
}
JSON
	mv "$STATE.tmp" "$STATE"
fi

if [[ ! -f "$ROOT/README.txt" ]]; then
	cat >"$ROOT/README.txt" <<'TEXT'
这是「SSR 多人协作版」的共享盘目录（本机演练用假目录）。
目录说明：
  published/ra|eama|udi   各部门发布的全量数据快照（<version>.csv + .manifest.json + 原始文件）
  state/state.json        各来源当前版本 + 最后一次整合记录（程序读写）
  state/state.lock        短临界区（程序自动创建/删除）
  state/consolidate.lock  整合互斥（程序自动创建/删除）
  consolidated/<id>/      每次整合的结果文件与 summary.json
  audit/                  审计事件（按天 jsonl）

注意：不要手工改 state.json；演练失败时可以先备份它再重建。
TEXT
fi

echo "假共享盘已就绪: $ROOT"
find "$ROOT" -maxdepth 2 -mindepth 1 | sort | sed "s|^$ROOT|  |"
echo
echo "state.json 摘要："
if command -v python3 >/dev/null 2>&1; then
	python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));print("  generation =",d.get("generation"),"| sources =",list((d.get("sources") or {}).keys()))' "$STATE"
else
	grep -E '"generation"|"sources"' "$STATE" | sed 's/^/  /'
fi
