#!/usr/bin/env bash
# 在本机起多份"不同部门"的实例，让它们指向同一个假共享盘（P0 演练脚手架）。
#
# 每份实例都有独立的配置目录与数据目录：
#   <workspace>/<role>/config/   ← 复制运行包的配置 + 生成 workflow.yaml（share.root 指向假共享盘）
#   <workspace>/<role>/data/     ← 数据库、模板、导出（程序按 config 的父目录解析）
# 启动时用环境变量 UDI_CONFIG_DIR 指向该实例的 config，所以多个实例互不干扰。
#
# 用法：
#   ./scripts/ssr-share-launch.sh --share /tmp/ssr-share --prepare-only
#   ./scripts/ssr-share-launch.sh --share /tmp/ssr-share
#   ./scripts/ssr-share-launch.sh --share /tmp/ssr-share --roles ra,sc,admin
set -euo pipefail

usage() {
	cat <<'USAGE'
用法: ssr-share-launch.sh [--share <目录>] [--workspace <目录>] [--package <目录>]
                          [--app <可执行文件>] [--roles ra,sc] [--prepare-only]

  --share        假共享盘根目录（先跑 ssr-share-init.sh 建好）
  --workspace    实例目录的根（默认 /tmp/ssr-rehearsal）
  --package      运行包目录（默认 release/SingleSourceReady）
  --app          应用可执行文件（默认按运行包自动找）
  --roles        要准备/启动的角色（默认 ra,sc）
  --prepare-only 只准备目录与配置，不启动程序
USAGE
}

SHARE="/tmp/ssr-share"
WORKSPACE="/tmp/ssr-rehearsal"
PACKAGE="release/SingleSourceReady"
APP=""
ROLES="ra,sc"
PREPARE_ONLY=0

while [[ $# -gt 0 ]]; do
	case "$1" in
	--share)
		SHARE="${2:-}"
		shift 2
		;;
	--workspace)
		WORKSPACE="${2:-}"
		shift 2
		;;
	--package)
		PACKAGE="${2:-}"
		shift 2
		;;
	--app)
		APP="${2:-}"
		shift 2
		;;
	--roles)
		ROLES="${2:-}"
		shift 2
		;;
	--prepare-only)
		PREPARE_ONLY=1
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

for path in "$SHARE" "$WORKSPACE"; do
	if [[ "$path" != /* ]]; then
		echo "路径必须是绝对路径: $path" >&2
		exit 1
	fi
done
if [[ ! -f "$SHARE/state/state.json" ]]; then
	echo "共享盘还没有初始化: $SHARE" >&2
	echo "先跑: ./scripts/ssr-share-init.sh --root $SHARE" >&2
	exit 1
fi

PACKAGE="$(cd "$PACKAGE" && pwd)"
if [[ -z "$APP" ]]; then
	for candidate in \
		"$PACKAGE/SingleSourceReady.app/Contents/MacOS/ssr" \
		"$PACKAGE/SingleSourceReady.exe" \
		"$PACKAGE/ssr"; do
		if [[ -x "$candidate" ]]; then
			APP="$candidate"
			break
		fi
	done
fi
if [[ -z "$APP" ]]; then
	echo "找不到应用可执行文件；请用 --app 指定（或用 --prepare-only 跳过启动）" >&2
	ls -1 "$PACKAGE" 2>/dev/null | sed 's/^/  /' >&2
	exit 1
fi

# 样例配置（workflow.yaml.example / users.yaml.example）优先取运行包，回退到仓库 config/
EXAMPLES="$PACKAGE/config"
if [[ ! -f "$EXAMPLES/workflow.yaml.example" && -f "config/workflow.yaml.example" ]]; then
	EXAMPLES="config"
fi

IFS=',' read -r -a ROLE_LIST <<<"$ROLES"
mkdir -p "$WORKSPACE"

prepared=()
for role in "${ROLE_LIST[@]}"; do
	role="$(echo "$role" | tr -d '[:space:]')"
	case "$role" in
	ra | eama | udi | sc | admin) ;;
	*)
		echo "未知角色: $role（可选 ra|eama|udi|sc|admin）" >&2
		exit 1
		;;
	esac
	instance="$WORKSPACE/$role"
	mkdir -p "$instance/config" "$instance/data"

	# 四份业务配置 + 可选的 sample_mapping（跳过备份与 defaults 之外的临时文件）
	find "$PACKAGE/config" -maxdepth 1 -type f -name '*.yaml' ! -name 'workflow.yaml.example' ! -name 'users.yaml.example' \
		-exec cp -f {} "$instance/config/" \;
	if [[ -d "$PACKAGE/config/defaults" ]]; then
		rm -rf "$instance/config/defaults"
		cp -R "$PACKAGE/config/defaults" "$instance/config/defaults"
	fi

	# 本实例的 workflow.yaml：share.root 指向假共享盘，邮件保持 disabled
	if [[ -f "$EXAMPLES/workflow.yaml.example" ]]; then
		awk -v root="$SHARE" '
			BEGIN { replaced = 0 }
			/^  root: / { print "  root: \x27" root "\x27"; replaced = 1; next }
			{ print }
			END { if (!replaced) exit 3 }
		' "$EXAMPLES/workflow.yaml.example" >"$instance/config/workflow.yaml.tmp"
		mv "$instance/config/workflow.yaml.tmp" "$instance/config/workflow.yaml"
	else
		cat >"$instance/config/workflow.yaml" <<YAML
share:
  root: '$SHARE'
sync:
  auto: true
  require_before_consolidate: true
consolidation:
  skip_unchanged: true
email:
  transport: disabled
YAML
	fi

	# 人员名单：任何实例都能选到全部角色（P1 之后用于登录页）
	if [[ -f "$EXAMPLES/users.yaml.example" ]]; then
		cp -f "$EXAMPLES/users.yaml.example" "$instance/config/users.yaml"
	fi

	# 模板：导出/导入都按 config 的父目录找 data/template
	if [[ -d "$PACKAGE/data/template" ]]; then
		mkdir -p "$instance/data/template"
		find "$PACKAGE/data/template" -maxdepth 1 -type f -name '*.xlsx' -exec cp -f {} "$instance/data/template/" \;
	fi

	prepared+=("$role|$instance")
done

echo "实例目录已准备（共享盘: ${SHARE}）："
for entry in "${prepared[@]}"; do
	role="${entry%%|*}"
	instance="${entry#*|}"
	echo "  $role"
	echo "    config: $instance/config"
	echo "    data  : $instance/data"
done

if [[ "$PREPARE_ONLY" -eq 1 ]]; then
	echo
	echo "只准备不启动（--prepare-only）。启动命令示例："
	for entry in "${prepared[@]}"; do
		role="${entry%%|*}"
		instance="${entry#*|}"
		echo "  UDI_CONFIG_DIR='$instance/config' '$APP' &"
	done
	exit 0
fi

echo
echo "启动实例："
pids=()
for entry in "${prepared[@]}"; do
	role="${entry%%|*}"
	instance="${entry#*|}"
	UDI_CONFIG_DIR="$instance/config" "$APP" >"/tmp/ssr-rehearsal-$role.log" 2>&1 &
	pid=$!
	pids+=("$pid")
	echo "  $role  pid=$pid  log=/tmp/ssr-rehearsal-$role.log"
done

printf '%s\n' "${pids[@]}" >"$WORKSPACE/pids"
echo
echo "已启动 ${#pids[@]} 个实例。关闭：kill \$(cat '$WORKSPACE/pids')"
echo "提示：一台机器上所有实例的操作系统用户名相同，审计里会显示同一用户名、不同角色（演练可接受）。"
