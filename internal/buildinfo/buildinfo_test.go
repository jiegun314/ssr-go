package buildinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestR26GitDescribeIsTurnedIntoAVersionString(t *testing.T) {
	cases := map[string]string{
		"v0.1.24":                  "0.1.24",
		"0.1.24":                   "0.1.24",
		"v0.1.24-3-gb8d56f4":       "0.1.24+3.gb8d56f4",
		"v0.1.24-3-gb8d56f4-dirty": "0.1.24+3.gb8d56f4.dirty",
		"v0.1.24-dirty":            "0.1.24+dirty",
		"b8d56f4":                  "0.0.0+gb8d56f4",
		"b8d56f4-dirty":            "0.0.0+gb8d56f4.dirty",
		"v0.2.0-rc.1":              "0.2.0-rc.1",
		"v0.2.0-rc.1-dirty":        "0.2.0-rc.1+dirty",
		"1.2.3-2-gabc1234":         "1.2.3+2.gabc1234",
		"0.1.24+3.gb8d56f4.dirty":  "0.1.24+3.gb8d56f4.dirty",
		"":                         "0.0.0",
	}
	for described, expected := range cases {
		if got := NormalizeGitDescribe(described); got != expected {
			t.Errorf("NormalizeGitDescribe(%q) = %q; want %q", described, got, expected)
		}
	}
}

func TestR26EveryDescribeResultIsAVersionString(t *testing.T) {
	for _, described := range []string{
		"v0.1.24", "v0.1.24-3-gb8d56f4", "b8d56f4-dirty", "v0.2.0-rc.1", "",
	} {
		if !IsVersionString(NormalizeGitDescribe(described)) {
			t.Errorf("%q 归一后不是合法版本串", described)
		}
	}
}

func TestR26ALooseValueIsNotAcceptedAsAVersion(t *testing.T) {
	for _, value := range []any{"0.1", "one", "v0.1.23", "0.1.24+", "", nil, 0.1} {
		if IsVersionString(value) {
			t.Errorf("%#v 不该被当成版本号", value)
		}
	}
}

func TestR26ArtifactNamesAndLabelsOfABuild(t *testing.T) {
	info := NewBuildInfo("0.1.24+3.gb8d56f4.dirty", "b8d56f4", "", "")
	if info.ArtifactVersion() != "0.1.24-3.gb8d56f4.dirty" {
		t.Errorf("ArtifactVersion = %q", info.ArtifactVersion())
	}
	if info.ArtifactDate() != "unknown" {
		t.Errorf("ArtifactDate = %q", info.ArtifactDate())
	}
	if info.Label() != "0.1.24+3.gb8d56f4.dirty (b8d56f4)" {
		t.Errorf("Label = %q", info.Label())
	}

	release := NewBuildInfo("0.1.24", "b8d56f4", "2026-10-01T14:32:05+08:00", SourceLdflags)
	if release.ArtifactDate() != "20261001" {
		t.Errorf("ArtifactDate = %q; want 20261001", release.ArtifactDate())
	}
	if release.NumericVersion() != [4]int{0, 1, 24, 0} {
		t.Errorf("NumericVersion = %v", release.NumericVersion())
	}
	if release.Detail() != "commit b8d56f4, build 2026-10-01T14:32:05+08:00" {
		t.Errorf("Detail = %q", release.Detail())
	}
}

func TestR26BuildConstantsWinAndSkipGit(t *testing.T) {
	originalVersion, originalCommit, originalDate := BuildVersion, BuildCommit, BuildDate
	defer func() {
		BuildVersion, BuildCommit, BuildDate = originalVersion, originalCommit, originalDate
	}()
	BuildVersion, BuildCommit, BuildDate = "9.9.9", "abc1234", "2026-10-01T14:32:05+08:00"

	// 即使给了一个不存在的目录，构建期常量也应该直接生效（不查 git）
	info := ReadBuildInfo(Options{ProjectRoot: filepath.Join(t.TempDir(), "no-such-repo")})

	if info.Version != "9.9.9" || info.Commit != "abc1234" {
		t.Fatalf("构建期常量应优先：%+v", info)
	}
	if info.Source != SourceLdflags {
		t.Errorf("Source = %q; want %q", info.Source, SourceLdflags)
	}
	if info.Label() != "9.9.9 (abc1234)" {
		t.Errorf("Label = %q", info.Label())
	}
}

func TestR26FallsBackToTheConfiguredVersion(t *testing.T) {
	originalVersion := BuildVersion
	defer func() { BuildVersion = originalVersion }()
	BuildVersion = ""

	info := ReadBuildInfo(Options{
		ProjectRoot: filepath.Join(t.TempDir(), "no-such-repo"),
		AppVersion:  "0.2.1",
	})

	if info.Version != "0.2.1" || info.Source != SourceSetting {
		t.Fatalf("应回退到 setting.yaml 的 APP_VERSION：%+v", info)
	}
	if info.Detail() != "version from config/setting.yaml" {
		t.Errorf("Detail = %q", info.Detail())
	}
}

func TestR26WithoutAnySourceTheVersionIsZero(t *testing.T) {
	originalVersion := BuildVersion
	defer func() { BuildVersion = originalVersion }()
	BuildVersion = ""

	info := ReadBuildInfo(Options{ProjectRoot: filepath.Join(t.TempDir(), "no-such-repo")})

	if info.Version != DefaultVersion || info.Source != SourceDefault {
		t.Fatalf("没有来源时报 0.0.0：%+v", info)
	}
	if info.Detail() != "version from "+SourceDefault {
		t.Errorf("Detail = %q", info.Detail())
	}
}

func TestR26AFrozenApplicationNeverAsksGit(t *testing.T) {
	originalVersion := BuildVersion
	defer func() { BuildVersion = originalVersion }()
	BuildVersion = ""

	// 指向一个真实仓库；Frozen 必须让它被忽略
	repository := initRepository(t, "v0.1.24")
	info := ReadBuildInfo(Options{
		ProjectRoot: repository,
		AppVersion:  "0.2.1",
		Frozen:      true,
	})

	if info.Source != SourceSetting || info.Version != "0.2.1" {
		t.Fatalf("发布包不查 git：%+v", info)
	}
}

func TestR26ATaggedCheckoutReportsTheTag(t *testing.T) {
	repository := initRepository(t, "v0.1.24")
	commit := gitOutput(t, repository, "rev-parse", "--short", "HEAD")

	info := ReadBuildInfo(Options{ProjectRoot: repository})

	if info.Version != "0.1.24" || info.Commit != commit || info.Source != SourceGit {
		t.Fatalf("带 tag 的工作区应报 tag：%+v（commit 应为 %s）", info, commit)
	}
}

func TestR26CommitsAfterTheTagBecomeBuildMetadata(t *testing.T) {
	repository := initRepository(t, "v0.1.24")
	gitOutput(t, repository, "commit", "-q", "--allow-empty", "-m", "second")

	info := ReadBuildInfo(Options{ProjectRoot: repository})

	if !strings.HasPrefix(info.Version, "0.1.24+1.g") {
		t.Fatalf("tag 之后的提交是 build metadata：%+v", info)
	}
	if !IsVersionString(info.Version) {
		t.Errorf("%q 必须是合法版本串", info.Version)
	}
}

func TestR26ACheckoutWithoutAVersionTagStillReportsAVersion(t *testing.T) {
	repository := initRepository(t, "")
	commit := gitOutput(t, repository, "rev-parse", "--short", "HEAD")

	info := ReadBuildInfo(Options{ProjectRoot: repository})

	if info.Version != "0.0.0+g"+commit {
		t.Fatalf("没有 tag 时报 commit：%+v", info)
	}
	if info.Commit != commit {
		t.Errorf("Commit = %q; want %q", info.Commit, commit)
	}
}

func TestR26AModifiedWorkingTreeIsMarkedDirty(t *testing.T) {
	repository := initRepository(t, "v0.1.24")
	tracked := filepath.Join(repository, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}

	info := ReadBuildInfo(Options{ProjectRoot: repository})

	if info.Version != "0.1.24+dirty" {
		t.Fatalf("脏工作区要带 .dirty：%+v", info)
	}
}

func TestR26SourceDateEpochPinsTheBuildDate(t *testing.T) {
	pinned, ok := BuildDateFromEnvironment("1600000000")
	if !ok {
		t.Fatal("合法的 SOURCE_DATE_EPOCH 应被接受")
	}
	if !strings.HasPrefix(pinned, "2020-09-13") {
		t.Errorf("SOURCE_DATE_EPOCH 决定的是时刻：%q", pinned)
	}
	if _, ok := BuildDateFromEnvironment("yesterday"); ok {
		t.Error("非法的 SOURCE_DATE_EPOCH 必须让构建停下来")
	}
}

func initRepository(t *testing.T, tag string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("需要 git 才能描述源码工作区")
	}
	repository := t.TempDir()
	gitOutput(t, repository, "init", "-q")
	gitOutput(t, repository, "config", "user.email", "ssr@example.com")
	gitOutput(t, repository, "config", "user.name", "SSR tests")
	if err := os.WriteFile(
		filepath.Join(repository, "tracked.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
	gitOutput(t, repository, "add", "tracked.txt")
	gitOutput(t, repository, "commit", "-q", "-m", "first")
	if tag != "" {
		gitOutput(t, repository, "tag", tag)
	}
	return repository
}

func gitOutput(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v 失败：%v", arguments, err)
	}
	return strings.TrimSpace(string(output))
}
