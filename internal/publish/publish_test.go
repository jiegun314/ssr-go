package publish

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

func testRows() []store.Row {
	return []store.Row{
		{"material_code": "M1", "product_name": "名称,带逗号"},
		{"material_code": "M2", "product_name": "名称2"},
	}
}

func testOptions(root string) Options {
	return Options{
		Root:    root,
		Source:  "ra_input",
		Columns: []string{"material_code", "product_name"},
		Rows:    testRows(),
		Owner:   share.Owner{Name: "zhangsan", Role: "ra", Username: "zhangsan", Machine: "PC-RA"},
		Now:     time.Date(2026, 11, 1, 9, 30, 0, 0, time.Local),
	}
}

func TestPublishWritesSnapshotAndState(t *testing.T) {
	root := t.TempDir()
	snapshot, err := Publish(testOptions(root))
	if err != nil {
		t.Fatalf("发布失败：%v", err)
	}
	if !strings.HasPrefix(snapshot.Version, "20261101T093000-") {
		t.Errorf("版本号应当以时间戳开头：%s", snapshot.Version)
	}
	content, err := os.ReadFile(snapshot.CSVPath)
	if err != nil {
		t.Fatalf("读快照失败：%v", err)
	}
	reader := csv.NewReader(strings.NewReader(string(content)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("快照 CSV 不合法：%v", err)
	}
	if len(records) != 3 || records[0][0] != "material_code" || records[1][1] != "名称,带逗号" {
		t.Fatalf("快照内容不对：%v", records)
	}
	// sha256 必须与文件内容一致
	if snapshot.SHA256 == "" || len(snapshot.SHA256) != 64 {
		t.Errorf("sha256 不对：%q", snapshot.SHA256)
	}
	manifestBytes, err := os.ReadFile(snapshot.ManifestPath)
	if err != nil {
		t.Fatalf("读 manifest 失败：%v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("manifest 不合法：%v", err)
	}
	if manifest.Source != "ra_input" || manifest.Rows != 2 || manifest.SHA256 != snapshot.SHA256 {
		t.Errorf("manifest 内容不对：%+v", manifest)
	}
	if manifest.PublishedBy.Name != "zhangsan" || manifest.PublishedBy.Role != "ra" {
		t.Errorf("manifest 应当记录发布人：%+v", manifest.PublishedBy)
	}

	// state.json：来源版本已记上、generation +1
	state, err := share.LoadState(root)
	if err != nil {
		t.Fatalf("读状态失败：%v", err)
	}
	version, ok := state.Sources["ra_input"]
	if !ok || version.Version != snapshot.Version || version.Rows != 2 {
		t.Fatalf("state 里没有正确的来源版本：%+v", state.Sources)
	}
	if state.Generation != 1 {
		t.Errorf("generation = %d；want 1", state.Generation)
	}
}

func TestPublishTwiceBumpsGenerationAndKeepsOldVersion(t *testing.T) {
	root := t.TempDir()
	first, err := Publish(testOptions(root))
	if err != nil {
		t.Fatalf("第一次发布失败：%v", err)
	}
	options := testOptions(root)
	options.Rows = append(options.Rows, store.Row{"material_code": "M3", "product_name": "名称3"})
	options.Now = options.Now.Add(time.Minute)
	second, err := Publish(options)
	if err != nil {
		t.Fatalf("第二次发布失败：%v", err)
	}
	if first.Version == second.Version {
		t.Error("内容变了，版本号应当变")
	}
	if _, err := os.Stat(first.CSVPath); err != nil {
		t.Errorf("旧版本快照应当保留：%v", err)
	}
	state, _ := share.LoadState(root)
	if state.Generation != 2 {
		t.Errorf("generation = %d；want 2", state.Generation)
	}
	if state.Sources["ra_input"].Rows != 3 {
		t.Errorf("当前版本应当是 3 行：%+v", state.Sources["ra_input"])
	}
}

func TestPublishKeepsOriginalWorkbook(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(t.TempDir(), "source.xlsx")
	if err := os.WriteFile(original, []byte("fake-xlsx"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := testOptions(root)
	options.OriginalPath = original
	snapshot, err := Publish(options)
	if err != nil {
		t.Fatalf("发布失败：%v", err)
	}
	if snapshot.OriginalWarning != "" {
		t.Errorf("原始文件应当能留档：%s", snapshot.OriginalWarning)
	}
	archived := filepath.Join(filepath.Dir(snapshot.CSVPath), snapshot.Version+".xlsx")
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("原始文件应当留档：%v", err)
	}
}

func TestPublishRejectsIncompleteOptions(t *testing.T) {
	if _, err := Publish(Options{}); err == nil {
		t.Error("没有共享盘路径应当报错")
	}
	options := testOptions(t.TempDir())
	options.Columns = nil
	if _, err := Publish(options); err == nil {
		t.Error("没有列应当报错")
	}
}
