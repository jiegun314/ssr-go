// Package buildinfo 说明运行中的程序是哪个版本（R26）。
//
// 版本号的唯一真源是 git tag：发布构建用
// `-ldflags "-X .../internal/buildinfo.BuildVersion=... -X ...BuildCommit=... -X ...BuildDate=..."`
// 把 build 期信息写进二进制（等价于 Python 版的 core/_build_info.py）。运行期按
// 「构建期常量 → git → setting.yaml 的 APP_VERSION」回退；APP_VERSION 只是回退值，
// BUILD_DATE 已删除，构建日期由构建过程产生。
package buildinfo

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Unknown 是「没有这个信息」的占位值。
const Unknown = "unknown"

// DefaultVersion 是任何信息都拿不到时报的版本。
const DefaultVersion = "0.0.0"

// 版本来源，用于「关于」窗口的 tooltip（与 Python 版的用户可见文案保持一致）。
const (
	SourceLdflags = "build constants (ldflags)"
	SourceGit     = "git"
	SourceSetting = "config/setting.yaml"
	SourceDefault = "defaults"
)

// 构建期由 -ldflags -X 注入；源码运行时保持为空。
var (
	BuildVersion = ""
	BuildCommit  = ""
	BuildDate    = ""
)

var (
	// 语义化版本，可带 pre-release 与 build metadata。
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	// `git describe --tags --dirty --always --match "v[0-9]*"` 的输出，前导 v 可省。
	describePattern = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(?:-(.+))?$`)
	// describe 结果里「tag 之后 N 个提交，位于哪个 commit」那一段。
	describeCommitsPattern = regexp.MustCompile(`^(\d+)-g([0-9a-fA-F]+)$`)
	// 文件名里不能出现的字符。
	artifactCharacterPattern = regexp.MustCompile(`[^0-9A-Za-z._-]`)
	datePattern              = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)
	numericPattern           = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)
)

// IsVersionString 判断一个配置值或生成值能不能当版本号显示。
func IsVersionString(value any) bool {
	text, ok := value.(string)
	return ok && versionPattern.MatchString(strings.TrimSpace(text))
}

// NormalizeGitDescribe 把 `git describe` 的结果变成版本串。
//
//	v0.1.24            → 0.1.24
//	v0.1.24-3-gb8d56f4 → 0.1.24+3.gb8d56f4   （tag 之后的提交是 build metadata）
//	v0.1.24-dirty      → 0.1.24+dirty
//	v0.2.0-rc.1        → 0.2.0-rc.1          （pre-release 保持 pre-release）
//	b8d56f4            → 0.0.0+gb8d56f4      （还没有任何版本 tag）
//	""                 → 0.0.0
func NormalizeGitDescribe(describeOutput string) string {
	text := strings.TrimSpace(describeOutput)
	dirty := strings.HasSuffix(text, "-dirty")
	if dirty {
		text = strings.TrimSuffix(text, "-dirty")
	}

	metadata := []string{}
	version := DefaultVersion
	if match := describePattern.FindStringSubmatch(text); match != nil {
		version = match[1]
		if suffix := match[2]; suffix != "" {
			if commits := describeCommitsPattern.FindStringSubmatch(suffix); commits != nil {
				metadata = append(metadata, commits[1]+".g"+commits[2])
			} else {
				version = version + "-" + suffix
			}
		}
	} else if IsVersionString(text) {
		// 已经是版本串（用 --version 固定的重打包）：原样保留，归一两次不会变样
		version = text
	} else if text != "" {
		// 还没有匹配的 tag：--always 只打印 commit
		metadata = append(metadata, "g"+strings.ReplaceAll(text, "-", "."))
	}
	if dirty {
		metadata = append(metadata, "dirty")
	}
	if len(metadata) > 0 {
		return version + "+" + strings.Join(metadata, ".")
	}
	return version
}

// BuildInfo 是一次构建的版本、commit 与构建日期。
type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
	Source    string
}

// NewBuildInfo 用回退值补齐未提供的字段。
func NewBuildInfo(version, commit, buildDate, source string) BuildInfo {
	if commit == "" {
		commit = Unknown
	}
	if buildDate == "" {
		buildDate = Unknown
	}
	if source == "" {
		source = SourceDefault
	}
	return BuildInfo{Version: version, Commit: commit, BuildDate: buildDate, Source: source}
}

// Label 是「版本 (commit)」，用于 exe 元数据与一行式日志。
func (info BuildInfo) Label() string {
	if info.Commit == Unknown {
		return info.Version
	}
	return info.Version + " (" + info.Commit + ")"
}

// ArtifactVersion 是可以用在文件名/目录名里的版本（「+」换成「-」）。
func (info BuildInfo) ArtifactVersion() string {
	return artifactCharacterPattern.ReplaceAllString(info.Version, "-")
}

// ArtifactDate 是 YYYYMMDD 形式的构建日期，没有构建日期时是 "unknown"。
func (info BuildInfo) ArtifactDate() string {
	match := datePattern.FindStringSubmatch(info.BuildDate)
	if match == nil {
		return Unknown
	}
	return match[1] + match[2] + match[3]
}

// NumericVersion 是 Windows 版本资源要的四个数字。
func (info BuildInfo) NumericVersion() [4]int {
	match := numericPattern.FindStringSubmatch(info.Version)
	if match == nil {
		return [4]int{0, 0, 0, 0}
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return [4]int{major, minor, patch, 0}
}

// Detail 是 commit 与构建日期，或版本的来源（关于窗口的 tooltip）。
func (info BuildInfo) Detail() string {
	parts := []string{}
	if info.Commit != Unknown {
		parts = append(parts, "commit "+info.Commit)
	}
	if info.BuildDate != Unknown {
		parts = append(parts, "build "+info.BuildDate)
	}
	if len(parts) == 0 {
		parts = append(parts, "version from "+info.Source)
	}
	return strings.Join(parts, ", ")
}

// Options 决定 ReadBuildInfo 能问到哪些来源。
type Options struct {
	// ProjectRoot 是 git 查询的工作目录（打包后没有仓库，会自然回退）。
	ProjectRoot string
	// AppVersion 是 config/setting.yaml 的 APP_VERSION（调用方读配置后传进来，
	// 这样本包不需要 YAML 依赖）。
	AppVersion string
	// Frozen 为 true 表示这是发布包：不查 git（包里没有仓库）。
	Frozen bool
}

// ReadBuildInfo 描述这一次运行：构建期常量 → git → setting.yaml → 默认值。
func ReadBuildInfo(options Options) BuildInfo {
	if IsVersionString(BuildVersion) {
		return NewBuildInfo(
			strings.TrimSpace(BuildVersion), BuildCommit, BuildDate, SourceLdflags)
	}
	if !options.Frozen {
		if fromGit, ok := readGit(options.ProjectRoot); ok {
			return fromGit
		}
	}
	if IsVersionString(options.AppVersion) {
		return NewBuildInfo(strings.TrimSpace(options.AppVersion), "", "", SourceSetting)
	}
	return NewBuildInfo(DefaultVersion, "", "", SourceDefault)
}

// readGit 用 git describe 描述工作区；没有 git 或没有仓库时返回 ok=false。
func readGit(projectRoot string) (BuildInfo, bool) {
	root := projectRoot
	if root == "" {
		root = SourceProjectRoot()
	}
	describe, ok := RunGit(root, "describe", "--tags", "--dirty", "--always", "--match", "v[0-9]*")
	if !ok || describe == "" {
		return BuildInfo{}, false
	}
	commit, _ := RunGit(root, "rev-parse", "--short", "HEAD")
	if commit == "" {
		commit = Unknown
	}
	return NewBuildInfo(NormalizeGitDescribe(describe), commit, "", SourceGit), true
}

// RunGit 跑一条 git 命令并返回 stdout；git 不存在或退出码非 0 时 ok=false。
// 空输出是合法答案（干净工作区的 `git status --porcelain` 什么都不打印）。
func RunGit(projectRoot string, arguments ...string) (string, bool) {
	command := exec.Command("git", arguments...)
	command.Dir = projectRoot
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

// SourceProjectRoot 在源码运行时给出模块根目录（可执行文件所在目录的父目录没有意义，
// 因为 `go run` 的可执行文件在临时目录里）。
func SourceProjectRoot() string {
	executable, err := os.Executable()
	if err != nil {
		workingDirectory, _ := os.Getwd()
		return workingDirectory
	}
	if strings.HasPrefix(executable, os.TempDir()) {
		workingDirectory, _ := os.Getwd()
		return workingDirectory
	}
	return executable
}

// BuildDateFromEnvironment 让 CI 用 SOURCE_DATE_EPOCH 固定构建时刻（R26）。
// 传入的时刻不是合法整数时返回 ok=false，调用方应当直接失败而不是静默取当前时间。
func BuildDateFromEnvironment(epoch string) (string, bool) {
	if strings.TrimSpace(epoch) == "" {
		return "", true
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(epoch), 10, 64)
	if err != nil {
		return "", false
	}
	moment := time.Unix(seconds, 0).In(time.Local)
	return moment.Format(time.RFC3339), true
}
