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
	if err == nil && !strings.HasPrefix(executable, os.TempDir()) {
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
