package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/buildinfo"
	"github.com/jiegun314/ssr-go/internal/config"
	"github.com/jiegun314/ssr-go/internal/store"
)

// releaseDirName 是发布目录名（= 产品名，版本之间固定不变，现场快捷方式与原地升级不受影响）。
const releaseDirName = "SingleSourceReady"

// releaseBlockers 是发布构建的前置守卫（R26）：HEAD 必须有 tag、工作区必须干净。
// allowDirty 只放行脏工作区（版本会带 .dirty），**不**放行未打 tag 的提交。
func releaseBlockers(projectRoot string, allowDirty bool) []string {
	blockers := []string{}
	describe, ok := buildinfo.RunGit(projectRoot, "describe", "--tags", "--always", "--match", "v[0-9]*")
	if !ok || describe == "" {
		return append(blockers, "HEAD is not tagged: run `git tag -a v<major>.<minor>.<patch> -m <message>` first")
	}
	// --always 在没有匹配 tag 时会退回裸 commit（归一化后是 0.0.0+g<hash>），
	// 那种情况同样算「没有 tag」，不能当成可发布的版本。
	if strings.HasPrefix(buildinfo.NormalizeGitDescribe(describe), "0.0.0") {
		return append(blockers, "HEAD is not tagged: run `git tag -a v<major>.<minor>.<patch> -m <message>` first")
	}
	if strings.HasSuffix(describe, "-dirty") && !allowDirty {
		status, _ := buildinfo.RunGit(projectRoot, "status", "--porcelain")
		blockers = append(blockers, "the working tree has uncommitted changes: "+strings.Join(strings.Fields(status), ", "))
	}
	return blockers
}

// releaseVersion 是这次发布构建的版本信息（构建期常量与产物命名共用同一份）。
func releaseVersion(projectRoot string, allowDirty bool, buildDate string) (buildinfo.BuildInfo, error) {
	if blockers := releaseBlockers(projectRoot, allowDirty); len(blockers) > 0 {
		return buildinfo.BuildInfo{}, fmt.Errorf("release build blocked:\n- %s", strings.Join(blockers, "\n- "))
	}
	describe, _ := buildinfo.RunGit(projectRoot, "describe", "--tags", "--dirty", "--always", "--match", "v[0-9]*")
	commit, _ := buildinfo.RunGit(projectRoot, "rev-parse", "--short", "HEAD")
	return buildinfo.NewBuildInfo(
		buildinfo.NormalizeGitDescribe(describe), commit, buildDate, buildinfo.SourceGit), nil
}

// releaseArtifactName 是压缩包名：SingleSourceReady-<版本>-<构建日期>.zip（R26）。
func releaseArtifactName(info buildinfo.BuildInfo) string {
	return fmt.Sprintf("%s-%s-%s.zip", releaseDirName, info.ArtifactVersion(), info.ArtifactDate())
}

func runReleaseFlags(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config", "", "配置目录（默认取 UDI_CONFIG_DIR 或可执行文件同级的 config/）")
	outDir := flags.String("out", "release", "发布产物目录")
	skipBuild := flags.Bool("skip-build", false, "跳过 wails build，用已有的 build/bin 产物组装")
	allowDirty := flags.Bool("allow-dirty", false, "本地自测：允许脏工作区（版本会带 .dirty）")
	noZip := flags.Bool("no-zip", false, "不生成压缩包")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	loader, err := config.NewLoader(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	projectRoot := loader.Resolver.ProjectRoot
	buildDate, ok := buildinfo.BuildDateFromEnvironment(os.Getenv("SOURCE_DATE_EPOCH"))
	if !ok {
		fmt.Fprintln(stderr, "SOURCE_DATE_EPOCH must be an integer number of seconds")
		return 1
	}
	if buildDate == "" {
		buildDate = time.Now().Format(time.RFC3339)
	}
	info, err := releaseVersion(projectRoot, *allowDirty, buildDate)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	ldflags := fmt.Sprintf(
		"-X github.com/jiegun314/ssr-go/internal/buildinfo.BuildVersion=%s "+
			"-X github.com/jiegun314/ssr-go/internal/buildinfo.BuildCommit=%s "+
			"-X github.com/jiegun314/ssr-go/internal/buildinfo.BuildDate=%s",
		info.Version, info.Commit, info.BuildDate)
	fmt.Fprintf(stdout, "Version: %s\nBuild date: %s\n", info.Label(), info.BuildDate)

	if !*skipBuild {
		command := exec.Command("wails", "build", "-ldflags", ldflags)
		command.Dir = projectRoot
		command.Stdout = stdout
		command.Stderr = stderr
		if err := command.Run(); err != nil {
			fmt.Fprintf(stderr, "wails build failed: %v\n", err)
			return 1
		}
	}

	releaseRoot := filepath.Join(projectRoot, *outDir, releaseDirName)
	if err := os.RemoveAll(releaseRoot); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if err := copyBuiltArtifact(projectRoot, releaseRoot); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	for _, directory := range []string{"config", filepath.Join("data", "template"), "resource"} {
		if err := copyTree(filepath.Join(projectRoot, directory), filepath.Join(releaseRoot, directory)); err != nil {
			fmt.Fprintf(stderr, "复制 %s 失败：%v\n", directory, err)
			return 1
		}
	}
	if err := os.MkdirAll(filepath.Join(releaseRoot, "output", "export"), 0o755); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if err := prepareReleaseDatabase(loader, releaseRoot); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(releaseRoot, "VERSION"), []byte(fmt.Sprintf(
		"version %s\ncommit %s\nbuild %s\n", info.Version, info.Commit, info.BuildDate)), 0o644); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Release ready: %s\n", releaseRoot)
	if !*noZip {
		archive, err := zipRelease(releaseRoot, releaseArtifactName(info))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Archive: %s\n", archive)
	}
	return 0
}

// copyBuiltArtifact 把 wails 的产物放进发布目录：
// macOS 是整个 .app，Windows 是 ssr.exe（可执行文件名在版本之间固定）。
func copyBuiltArtifact(projectRoot string, releaseRoot string) error {
	bin := filepath.Join(projectRoot, "build", "bin")
	switch runtime.GOOS {
	case "darwin":
		source := filepath.Join(bin, releaseDirName+".app")
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("找不到 %s：先运行 wails build（或去掉 --skip-build）", source)
		}
		return copyTree(source, filepath.Join(releaseRoot, releaseDirName+".app"))
	case "windows":
		source := filepath.Join(bin, "ssr.exe")
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("找不到 %s：先运行 wails build（或去掉 --skip-build）", source)
		}
		content, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(releaseRoot, "ssr.exe"), content, 0o755)
	default:
		return fmt.Errorf("不支持的平台：%s", runtime.GOOS)
	}
}

// prepareReleaseDatabase 生成发布包用的空库（只含空的 operation_log）。
func prepareReleaseDatabase(loader *config.Loader, releaseRoot string) error {
	setting, err := loader.LoadSetting()
	if err != nil {
		return err
	}
	database, _ := setting["database"].(map[string]any)
	databasePath := filepath.Join(releaseRoot, textValue(database["path"]))
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(databasePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	repository, err := store.Open(databasePath)
	if err != nil {
		return err
	}
	defer repository.Close()
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	identityFields, err := loader.LoadDuplicateCheckIdentityFields()
	if err != nil {
		return err
	}
	logColumns, err := loader.LoadLogColumns()
	if err != nil {
		return err
	}
	_, err = store.OpenOperationLog(repository, store.OperationLogOptions{
		TableName:      textValue(operationLog["name"]),
		TimeColumn:     textValue(tables["operation_log_time_column"]),
		IdentityFields: identityFields,
		LogColumns:     logColumns,
	})
	return err
}

func copyTree(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := entry.Info(); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		return os.WriteFile(destination, content, mode)
	})
}

// zipRelease 打包发布目录里的**文件**（解压即得 run 脚本与可执行文件，而不是多一层目录）。
func zipRelease(releaseRoot string, archiveName string) (string, error) {
	archivePath := filepath.Join(filepath.Dir(releaseRoot), archiveName)
	file, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	defer writer.Close()
	err = filepath.WalkDir(releaseRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == releaseRoot {
			return nil
		}
		relative, err := filepath.Rel(releaseRoot, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if entry.IsDir() {
			_, err := writer.Create(name + "/")
			return err
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o644)
		target, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		_, err = io.Copy(target, source)
		return err
	})
	if err != nil {
		return "", err
	}
	return archivePath, nil
}
