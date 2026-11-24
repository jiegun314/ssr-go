package nmpa_test

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/nmpa"
	"github.com/jiegun314/ssr-go/internal/store"
)

const testCreditCode = "91310115703465170L"

func testDeviceXML(primaryDI, code, productName string) string {
	return `<device><zxxsdycpbs>` + primaryDI + `</zxxsdycpbs><tyshxydm>` + code +
		`</tyshxydm><spmc>` + productName + `</spmc><versionTime>2026-11-23 10:00:00</versionTime>` +
		`<deviceRecordKey>` + primaryDI + `</deviceRecordKey></device>`
}

func testArchive(t *testing.T, devices ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("UDID_INCREMENTAL_DOWNLOAD_PART1_Of_1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`<udid><devices>` + strings.Join(devices, "") + `</devices></udid>`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServiceSyncFullFiltersByCreditCode(t *testing.T) {
	zipPath := testArchive(t,
		testDeviceXML("DI-1", testCreditCode, "匹配产品"),
		testDeviceXML("DI-2", "OTHER", "其他企业"),
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rss":
			_, _ = writer.Write([]byte(`<rss><channel><title>NMPA</title>
<item><title>全量</title><link>` + "http://" + request.Host + `/UDID_FULL_RELEASE_20261101.zip</link><description>2 条</description></item>
</channel></rss>`))
		case "/UDID_FULL_RELEASE_20261101.zip":
			http.ServeFile(writer, request, zipPath)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	nmpaStore, err := store.OpenNMPA(filepath.Join(t.TempDir(), "nmpa.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer nmpaStore.Close()
	service := nmpa.NewService(nmpaStore, func() (string, error) { return testCreditCode, nil })
	service.FeedURL = server.URL + "/rss"
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.FullRelease == nil || status.FullRelease.FileName != "UDID_FULL_RELEASE_20261101.zip" {
		t.Fatalf("full release = %#v", status.FullRelease)
	}
	result, err := service.SyncFull(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Devices != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Status.LastFullDate != "2026-11-01" {
		t.Fatalf("full date = %q", result.Status.LastFullDate)
	}
	search, err := service.Query("", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 1 || search.Devices[0].PrimaryDI != "DI-1" {
		t.Fatalf("search = %#v", search)
	}
}

func TestServiceWeeklyRejectsGap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`<rss><channel>
<item><title>周更</title><link>http://` + request.Host + `/UDID_WEEKLY_UPDATE_20261116_20261122.zip</link><description>1 条</description></item>
</channel></rss>`))
	}))
	defer server.Close()

	nmpaStore, err := store.OpenNMPA(filepath.Join(t.TempDir(), "nmpa.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer nmpaStore.Close()
	if err := nmpaStore.SetMetadata(map[string]string{
		"social_credit_code": testCreditCode,
		"full_date":          "2026-11-01",
		"last_week_end":      "2026-11-01",
	}); err != nil {
		t.Fatal(err)
	}
	service := nmpa.NewService(nmpaStore, func() (string, error) { return testCreditCode, nil })
	service.FeedURL = server.URL + "/rss"
	_, err = service.SyncWeekly(context.Background())
	if err == nil || !strings.Contains(err.Error(), "断档") {
		t.Fatalf("expected gap error, got %v", err)
	}
}

func TestServiceWeeklyAppliesAfterBaseline(t *testing.T) {
	zipPath := testArchive(t, testDeviceXML("DI-W", testCreditCode, "周更产品"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rss":
			_, _ = writer.Write([]byte(`<rss><channel>
<item><title>周更</title><link>http://` + request.Host + `/UDID_WEEKLY_UPDATE_20261116_20261122.zip</link><description>1 条</description></item>
</channel></rss>`))
		case "/UDID_WEEKLY_UPDATE_20261116_20261122.zip":
			http.ServeFile(writer, request, zipPath)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	nmpaStore, err := store.OpenNMPA(filepath.Join(t.TempDir(), "nmpa.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer nmpaStore.Close()
	if err := nmpaStore.SetMetadata(map[string]string{
		"social_credit_code": testCreditCode,
		"full_date":          "2026-11-01",
		"last_week_end":      "2026-11-15",
	}); err != nil {
		t.Fatal(err)
	}
	service := nmpa.NewService(nmpaStore, func() (string, error) { return testCreditCode, nil })
	service.FeedURL = server.URL + "/rss"
	result, err := service.SyncWeekly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Devices != 1 {
		t.Fatalf("result = %#v", result)
	}
	status, err := nmpaStore.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if status["last_week_end"] != "2026-11-22" {
		t.Fatalf("last_week_end = %q", status["last_week_end"])
	}
}

func TestServiceWeeklyWaitsForNewFullRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`<rss><channel>
<item><title>全量</title><link>http://` + request.Host + `/UDID_FULL_RELEASE_20261101.zip</link><description>1 条</description></item>
<item><title>周更</title><link>http://` + request.Host + `/UDID_WEEKLY_UPDATE_20261116_20261122.zip</link><description>1 条</description></item>
</channel></rss>`))
	}))
	defer server.Close()

	nmpaStore, err := store.OpenNMPA(filepath.Join(t.TempDir(), "nmpa.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer nmpaStore.Close()
	if err := nmpaStore.SetMetadata(map[string]string{
		"social_credit_code": testCreditCode,
		"full_date":          "2026-10-01",
		"last_week_end":      "2026-11-15",
	}); err != nil {
		t.Fatal(err)
	}
	service := nmpa.NewService(nmpaStore, func() (string, error) { return testCreditCode, nil })
	service.FeedURL = server.URL + "/rss"
	_, err = service.SyncWeekly(context.Background())
	if err == nil || !strings.Contains(err.Error(), "新的全量包") {
		t.Fatalf("expected new-full error, got %v", err)
	}
}
