// Command ssr-core 是 SSR(SingleSource Ready) 的无界面命令行。
//
// 核心业务（导入 / 整合 / 变更检测 / 导出）必须能在没有界面的环境里跑，它既是
// golden diff 的载体，也是「夜间批量处理」这类未来需求的基础；Wails 只是最后一层。
//
// 子命令（AGENTS.md §11.1 第 1 条）：
//
//	import | consolidate | export | snapshot | genlogcolumns |
//	alignlogcolumns | gensample | preparedb
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jiegun314/ssr-go/internal/buildinfo"
	"github.com/jiegun314/ssr-go/internal/paths"
)

const usageText = `ssr-core —— SSR(SingleSource Ready) 无界面命令行

用法：
  ssr-core <子命令> [参数]

子命令：
  version           打印版本信息（R26）
  import            导入一份来源 Excel（--source <键> --file <路径>）
  consolidate       按配置整合四份来源（--config <目录>）
  export            把 Ready 行导出到 SingleSource 模板（--file <路径>）
  snapshot          导出逐格 dump，供 golden diff 使用（--out <目录>）
  genlogcolumns     由模板表头生成 config/log_columns.yaml
  alignlogcolumns   把旧库的 operation_log 对齐到当前列
  gensample         生成本地样本 Excel
  preparedb         生成发布包用的空数据库

全局参数：
  --config <目录>   指定配置目录（等价于环境变量 UDI_CONFIG_DIR，见 AGENTS.md §4.3）

除 version 外，各子命令仍在移植中：它们会打印可执行的用法与「尚未实现」。
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	switch arguments[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usageText)
		return 0
	case "version":
		return runVersion(arguments[1:], stdout, stderr)
	case "import":
		return runImportFlags(arguments[1:], stdout, stderr)
	case "consolidate":
		return runConsolidateFlags(arguments[1:], stdout, stderr)
	case "export":
		return runExportFlags(arguments[1:], stdout, stderr)
	case "snapshot":
		return runSnapshotFlags(arguments[1:], stdout, stderr)
	case "genlogcolumns", "alignlogcolumns", "gensample", "preparedb":
		return runNotImplemented(arguments[0], arguments[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "未知子命令：%s\n\n%s", arguments[0], usageText)
		return 2
	}
}

// runVersion 打印版本、commit 与构建日期（R26 的回退链）。
func runVersion(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR 或可执行文件同级的 config/）")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	resolver, err := paths.New(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "解析配置目录失败：%v\n", err)
		return 1
	}
	info := buildinfo.ReadBuildInfo(buildinfo.Options{
		ProjectRoot: resolver.ProjectRoot,
		Frozen:      isPackaged(),
	})
	fmt.Fprintf(stdout, "版本：%s\n", info.Label())
	fmt.Fprintf(stdout, "详情：%s\n", info.Detail())
	fmt.Fprintf(stdout, "配置：%s\n", resolver.ConfigDir)
	return 0
}

// runNotImplemented 定义每个子命令的对外接口（--help 可查），然后明确报「尚未实现」。
// 骨架阶段这样做的目的：接口先固定下来，实现时不会偷偷改变命令行契约。
func runNotImplemented(name string, arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stdout)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR）")
	switch name {
	case "import":
		flags.String("source", "", "来源键：ra_input | global_udi_input | medical_insurance_code | product_category")
		flags.String("file", "", "要导入的 Excel 文件")
	case "export":
		flags.String("file", "", "导出目标文件名（默认 = 模板名 + 时间戳，见 R20）")
	case "snapshot":
		flags.String("out", "baseline-go", "逐格 dump 的输出目录")
		flags.String("input", "data/input/sample/valid", "四份来源 Excel 所在目录")
	case "genlogcolumns":
		flags.Bool("check", false, "只校验 config/log_columns.yaml 与模板是否一致，不写文件")
	case "alignlogcolumns":
		flags.String("database", "", "要对齐的 SQLite 文件（默认取 setting.yaml）")
		flags.Bool("no-backup", false, "跳过备份（默认先备份）")
	case "gensample":
		flags.String("output", "data/input/sample", "样本输出目录")
		flags.Int("rows", 5, "产品代码数量")
	case "preparedb":
		flags.String("output", "", "空数据库输出路径")
	}
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	resolver, err := paths.New(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "解析配置目录失败：%v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "子命令 %s 尚未实现（配置目录：%s）\n", name, resolver.ConfigDir)
	fmt.Fprintf(stderr, "见 ssr_go/AGENTS.md §11.1 的交付清单与 ssr_go/baseline/ 的验收基准\n")
	return 2
}

// isPackaged 说明这次运行是不是发布包（发布包没有仓库，所以不查 git）。
func isPackaged() bool {
	// Wails 打包后会设置它；源码运行时为 false。
	return os.Getenv("SSR_PACKAGED") == "1"
}
