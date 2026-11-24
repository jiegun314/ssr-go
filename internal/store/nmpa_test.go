package store

import (
	"path/filepath"
	"testing"

	"github.com/jiegun314/ssr-go/internal/nmpa"
)

func TestNMPAStoreReplaceSearchAndUpsert(t *testing.T) {
	store, err := OpenNMPA(filepath.Join(t.TempDir(), "nmpa_udi.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	device := nmpa.Device{
		PrimaryDI: "DI-1", ProductName: "椎间融合器", DeviceGenericName: "融合器",
		Specification: "规格-A", RegistrantName: "强生（上海）医疗器材有限公司",
		RegistrationFilingCertificateNo: "国械注准20260001",
		UnifiedSocialCreditCode:         "91310115703465170L", VersionTime: "2026-11-23 10:00:00",
		PackageCount: 1, ContactCount: 1,
		Packages: []nmpa.Package{{PackageDI: "P1", PackageType: "盒", LowerLevelPackageQuantity: "1"}},
		Contacts: []nmpa.Contact{{Email: "qa@example.com", Phone: "123"}},
	}
	if err := store.ReplaceAll([]nmpa.Device{device}, map[string]string{"social_credit_code": "CODE"}); err != nil {
		t.Fatal(err)
	}
	devices, packages, contacts, err := store.Counts()
	if err != nil {
		t.Fatal(err)
	}
	if devices != 1 || packages != 1 || contacts != 1 {
		t.Fatalf("counts = %d/%d/%d", devices, packages, contacts)
	}
	result, err := store.Search("椎间", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Devices) != 1 || result.Devices[0].PrimaryDI != "DI-1" {
		t.Fatalf("search = %#v", result)
	}
	if result.Devices[0].PackageCount != 1 || result.Devices[0].ContactCount != 1 {
		t.Fatalf("embedded counts = %#v", result.Devices[0])
	}
	empty, err := store.Search("", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Total != 1 {
		t.Fatalf("empty search total = %d", empty.Total)
	}

	device.ProductName = "椎间融合器（更新）"
	device.Packages = nil
	device.Contacts = nil
	device.PackageCount = 0
	device.ContactCount = 0
	device.VersionTime = "2026-11-24 10:00:00"
	if err := store.Upsert([]nmpa.Device{device}, map[string]string{"last_week_end": "2026-11-24"}); err != nil {
		t.Fatal(err)
	}
	devices, packages, contacts, err = store.Counts()
	if err != nil {
		t.Fatal(err)
	}
	if devices != 1 || packages != 0 || contacts != 0 {
		t.Fatalf("after upsert counts = %d/%d/%d", devices, packages, contacts)
	}
	result, err = store.Search("更新", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Devices[0].ProductName != "椎间融合器（更新）" {
		t.Fatalf("updated search = %#v", result)
	}
	metadata, err := store.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if metadata["last_week_end"] != "2026-11-24" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
