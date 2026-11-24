package nmpa

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDeviceZip(t *testing.T, xmlBody string) string {
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
	if _, err := entry.Write([]byte(xmlBody)); err != nil {
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

func TestParseArchiveFiltersByCreditCodeAndMapsNestedData(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<udid><header frequency="daily" type="DELTA_INCREMENTAL"/><devices>
<device>
  <zxxsdycpbs>DI-100</zxxsdycpbs>
  <cpbsbmtxmc>GS1</cpbsbmtxmc>
  <spmc>椎间融合器</spmc>
  <cpmctymc>椎间融合器</cpmctymc>
  <ggxh>规格-A</ggxh>
  <tyshxydm> 91310115703465170l </tyshxydm>
  <zczbhhzbapzbh>国械注准20260001</zczbhhzbapzbh>
  <ylqxzcrbarmc>强生（上海）医疗器材有限公司</ylqxzcrbarmc>
  <bszt>一维码,二维码</bszt>
  <deviceRecordKey>KEY-1</deviceRecordKey>
  <versionNumber>1</versionNumber>
  <versionTime>2026-11-23 10:00:00</versionTime>
  <versionStauts>新增</versionStauts>
  <packingList><packing><bzcpbs>PKG-1</bzcpbs><cpbzjb>盒</cpbzjb><bznhxyjcpbssl>1</bznhxyjcpbssl><bznhxyjbzcpbs>DI-100</bznhxyjbzcpbs></packing><packing><bzcpbs>PKG-2</bzcpbs></packing></packingList>
  <contactList><contact><qylxryx>qa@example.com</qylxryx><qylxrdh>123</qylxrdh></contact></contactList>
</device>
<device>
  <zxxsdycpbs>DI-OTHER</zxxsdycpbs>
  <tyshxydm>OTHERCODE</tyshxydm>
  <spmc>其他企业</spmc>
</device>
<device>
  <zxxsdycpbs>DI-100</zxxsdycpbs>
  <spmc>椎间融合器（更新）</spmc>
  <tyshxydm>91310115703465170L</tyshxydm>
  <deviceRecordKey>KEY-2</deviceRecordKey>
  <versionNumber>2</versionNumber>
  <versionTime>2026-11-23 11:00:00</versionTime>
  <versionStauts>更新</versionStauts>
</device>
</devices></udid>`
	path := writeDeviceZip(t, body)
	parsed, err := ParseArchive(path, "91310115703465170L")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Total != 3 || parsed.Skipped != 1 || len(parsed.Devices) != 1 {
		t.Fatalf("parsed total=%d skipped=%d devices=%d", parsed.Total, parsed.Skipped, len(parsed.Devices))
	}
	device := parsed.Devices[0]
	if device.PrimaryDI != "DI-100" || device.ProductName != "椎间融合器（更新）" {
		t.Fatalf("device = %#v", device)
	}
	if device.VersionNumber != "2" || device.DeviceRecordKey != "KEY-2" {
		t.Fatalf("latest version not kept: %#v", device)
	}
	if device.PackageCount != 0 || device.ContactCount != 0 {
		t.Fatalf("duplicate row should replace child rows: %#v", device)
	}
	if device.UnifiedSocialCreditCode != "91310115703465170L" {
		t.Fatalf("credit code = %q", device.UnifiedSocialCreditCode)
	}
}

func TestParseArchiveKeepsPackagesAndContacts(t *testing.T) {
	body := `<udid><devices><device>
<zxxsdycpbs>DI-1</zxxsdycpbs><tyshxydm>CODE</tyshxydm><spmc>产品</spmc>
<packingList>
<packing><bzcpbs>P1</bzcpbs><cpbzjb>盒</cpbzjb><bznhxyjcpbssl>1</bznhxyjcpbssl><bznhxyjbzcpbs>DI-1</bznhxyjbzcpbs></packing>
<packing><bzcpbs>P2</bzcpbs><cpbzjb>箱</cpbzjb><bznhxyjcpbssl>10</bznhxyjcpbssl><bznhxyjbzcpbs>P1</bznhxyjbzcpbs></packing>
</packingList>
<contactList><contact><qylxrcz>021-1</qylxrcz><qylxryx>qa@example.com</qylxryx><qylxrdh>123</qylxrdh></contact></contactList>
</device></devices></udid>`
	parsed, err := ParseArchive(writeDeviceZip(t, body), "code")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Devices) != 1 {
		t.Fatalf("devices = %d", len(parsed.Devices))
	}
	device := parsed.Devices[0]
	if device.PackageCount != 2 || device.ContactCount != 1 {
		t.Fatalf("counts = %d/%d", device.PackageCount, device.ContactCount)
	}
	if device.Packages[0].PackageDI != "P1" || device.Packages[1].LowerLevelPackageQuantity != "10" {
		t.Fatalf("packages = %#v", device.Packages)
	}
	if device.Contacts[0].Email != "qa@example.com" {
		t.Fatalf("contacts = %#v", device.Contacts)
	}
}

func TestParseArchiveRequiresCodeAndXML(t *testing.T) {
	path := writeDeviceZip(t, `<udid><devices></devices></udid>`)
	if _, err := ParseArchive(path, ""); err == nil || !strings.Contains(err.Error(), "统一社会信用代码为空") {
		t.Fatalf("expected empty code error, got %v", err)
	}
	emptyZip := filepath.Join(t.TempDir(), "empty.zip")
	file, err := os.Create(emptyZip)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := ParseArchive(emptyZip, "CODE"); err == nil {
		t.Fatal("expected missing XML error")
	}
}

func TestParseArchiveDropsIllegalXMLControlCharacters(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<udid><devices><device>
<zxxsdycpbs>DI-1</zxxsdycpbs><spmc>椎间` + "\x02" + `融合器` + "\uFFFE" + `</spmc>
<tyshxydm>91310115703465170L</tyshxydm>
</device></devices></udid>`
	path := writeDeviceZip(t, xmlData)
	parsed, err := ParseArchive(path, "91310115703465170L")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Devices) != 1 || parsed.Devices[0].ProductName != "椎间融合器" {
		t.Fatalf("devices = %#v", parsed.Devices)
	}
}

func TestParseArchiveReadsNestedWeeklyZips(t *testing.T) {
	inner := `<?xml version="1.0" encoding="UTF-8"?>
<udid><devices><device><zxxsdycpbs>DI-WEEKLY</zxxsdycpbs><tyshxydm>CODE</tyshxydm><spmc>周更产品</spmc></device></devices></udid>`
	innerPath := writeDeviceZip(t, inner)
	innerBytes, err := os.ReadFile(innerPath)
	if err != nil {
		t.Fatal(err)
	}
	outerPath := filepath.Join(t.TempDir(), "weekly.zip")
	outerFile, err := os.Create(outerPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(outerFile)
	entry, err := writer.Create("UDID_DAY_UPDATE_20261102.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(innerBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := outerFile.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseArchive(outerPath, "CODE")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Devices) != 1 || parsed.Devices[0].PrimaryDI != "DI-WEEKLY" {
		t.Fatalf("devices = %#v", parsed.Devices)
	}
}
