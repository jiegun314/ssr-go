// Package paths 复刻 Python 版 PathResolver 的路径基准（AGENTS.md §4.3）。
//
// 规则：
//   - 配置目录可被环境变量 UDI_CONFIG_DIR 覆盖（测试就是这么隔离的）；
//   - project_root = 配置目录的父目录；
//   - 配置里的相对路径都相对 project_root，绝对路径原样使用；
//   - 打包后配置随可执行文件同级目录走，所以可执行文件所在目录就是 project_root。
//
// 因此设置 UDI_CONFIG_DIR 会同时改变数据库、模板与导出目录的解析基准 —— 这是刻意行为，
// 重写不许改。
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// ConfigEnvironmentVariable 是覆盖配置目录的环境变量名。
const ConfigEnvironmentVariable = "UDI_CONFIG_DIR"

// Resolver 提供一套「相对 project_root」的路径策略。
type Resolver struct {
	// ConfigDir 是四份 YAML 所在目录。
	ConfigDir string
	// ProjectRoot 是相对路径的解析基准（= ConfigDir 的父目录）。
	ProjectRoot string
}

// New 解析配置目录：显式参数 > UDI_CONFIG_DIR > 可执行文件同级的 config/。
func New(explicitConfigDir string) (Resolver, error) {
	configDir := explicitConfigDir
	if configDir == "" {
		configDir = os.Getenv(ConfigEnvironmentVariable)
	}
	if configDir == "" {
		// 源码运行（`go run`）时，可执行文件在 Go 构建缓存里，按它推导会把项目根算到缓存目录；
		// 所以先看当前工作目录里有没有 config/（仓库根就是这么用的），没有才按可执行文件推导
		// （打包产物走的就是后者：exe 同级，macOS 的 .app 则退到 .app 所在目录）。
		if workingDirectory, err := os.Getwd(); err == nil {
			candidate := filepath.Join(workingDirectory, "config")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				configDir = candidate
			}
		}
	}
	if configDir == "" {
		configDir = filepath.Join(baseDirectory(), "config")
	}
	absoluteConfigDir, err := filepath.Abs(configDir)
	if err != nil {
		return Resolver{}, err
	}
	return Resolver{
		ConfigDir:   absoluteConfigDir,
		ProjectRoot: filepath.Dir(absoluteConfigDir),
	}, nil
}

// Resolve 把配置里的路径解析成绝对路径。
func (resolver Resolver) Resolve(configuredPath string) string {
	if filepath.IsAbs(configuredPath) {
		return filepath.Clean(configuredPath)
	}
	return filepath.Join(resolver.ProjectRoot, configuredPath)
}

// BaseDirectory 是「程序自己所在的目录」：可执行文件同级（macOS 的 .app 则退到
// .app 所在目录），`go run` 这种可执行文件在临时目录里的情况退回当前工作目录。
// 配置装载层用它找发布包自带的默认配置（config/defaults/）。
func BaseDirectory() string { return baseDirectory() }

// ConfigFile 返回四份 YAML 之一在配置目录里的路径。
func (resolver Resolver) ConfigFile(name string) string {
	return filepath.Join(resolver.ConfigDir, name)
}

// baseDirectory 是「可执行文件同级目录」；`go run` 的可执行文件在系统临时目录里，
// 那种情况下改用当前工作目录，否则开发时会去临时目录找配置。
//
// macOS 的 .app 是个例外：可执行文件在 `X.app/Contents/MacOS/` 里，而发布约定是配置与
// 模板放在 **.app 同级**目录（现场改配置不用重新打包），所以要往上退三层回到 .app 的父目录。
func baseDirectory() string {
	executable, err := os.Executable()
	if err == nil && !insideTempDirectory(executable) {
		if directory, ok := bundleParent(executable); ok {
			return directory
		}
		return filepath.Dir(executable)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return workingDirectory
}

// insideTempDirectory 判断可执行文件是否在系统临时目录里（`go run` 的情形）。
//
// 必须先解析符号链接：macOS 上 os.TempDir() 通常是 /var/folders/...，而
// os.Executable() 返回的是同一个目录的 /private/var/folders/... 写法，
// 直接做前缀比较会漏判（漏判的后果是把项目根算到临时目录，配置与 git 都找不到）。
func insideTempDirectory(executable string) bool {
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		resolvedExecutable = executable
	}
	resolvedTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		resolvedTemp = os.TempDir()
	}
	return strings.HasPrefix(resolvedExecutable, resolvedTemp)
}

// bundleParent 判断可执行文件是否在 macOS 应用包里（.../X.app/Contents/MacOS/Y），
// 是则返回 .app 所在目录。
func bundleParent(executable string) (string, bool) {
	macOSDirectory := filepath.Dir(executable)        // .../X.app/Contents/MacOS
	contentsDirectory := filepath.Dir(macOSDirectory) // .../X.app/Contents
	bundle := filepath.Dir(contentsDirectory)         // .../X.app
	if filepath.Base(macOSDirectory) != "MacOS" ||
		filepath.Base(contentsDirectory) != "Contents" ||
		!strings.HasSuffix(bundle, ".app") {
		return "", false
	}
	return filepath.Dir(bundle), true
}
