package store

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/nmpa"
)

// NMPAStore 是药监参考库的独立 SQLite 访问层。
//
// 它和主库分开：药监数据只用于本地查询，不参与导入/整合；全量包按统一社会信用代码
// 过滤后才写入，因此这里永远只有一家企业的数据。
type NMPAStore struct {
	repository *Repository
}

// OpenNMPA 打开（必要时创建）药监参考库，并补齐表结构。
func OpenNMPA(path string) (*NMPAStore, error) {
	repository, err := Open(path)
	if err != nil {
		return nil, err
	}
	store := &NMPAStore{repository: repository}
	if err := store.ensureSchema(); err != nil {
		repository.Close()
		return nil, err
	}
	return store, nil
}

func (store *NMPAStore) ensureSchema() error {
	columns := make([]string, 0, len(nmpa.DeviceColumns)+3)
	for _, column := range nmpa.DeviceColumns {
		columns = append(columns, quoteIdentifier(column)+" TEXT NOT NULL DEFAULT ''")
	}
	columns = append(columns, "package_count INTEGER NOT NULL DEFAULT 0", "contact_count INTEGER NOT NULL DEFAULT 0", "updated_at TEXT NOT NULL DEFAULT ''")
	statements := []string{
		`CREATE TABLE IF NOT EXISTS nmpa_devices (` + strings.Join(columns, ",") + `, PRIMARY KEY (primary_di))`,
		`CREATE INDEX IF NOT EXISTS idx_nmpa_devices_record_key ON nmpa_devices(device_record_key)`,
		`CREATE INDEX IF NOT EXISTS idx_nmpa_devices_certificate ON nmpa_devices(registration_filing_certificate_number)`,
		`CREATE INDEX IF NOT EXISTS idx_nmpa_devices_registrant ON nmpa_devices(registrant_name)`,
		`CREATE INDEX IF NOT EXISTS idx_nmpa_devices_classification ON nmpa_devices(classification_code)`,
		`CREATE TABLE IF NOT EXISTS nmpa_packages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			primary_di TEXT NOT NULL,
			package_di TEXT NOT NULL DEFAULT '',
			package_type TEXT NOT NULL DEFAULT '',
			lower_level_package_quantity TEXT NOT NULL DEFAULT '',
			lower_level_package_identifier TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nmpa_packages_device ON nmpa_packages(primary_di)`,
		`CREATE TABLE IF NOT EXISTS nmpa_contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			primary_di TEXT NOT NULL,
			fax TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nmpa_contacts_device ON nmpa_contacts(primary_di)`,
		`CREATE TABLE IF NOT EXISTS nmpa_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, statement := range statements {
		if _, err := store.repository.db.Exec(statement); err != nil {
			return fmt.Errorf("创建药监参考库失败: %w", err)
		}
	}
	return nil
}

// Close 关闭参考库连接。
func (store *NMPAStore) Close() error {
	if store == nil || store.repository == nil {
		return nil
	}
	return store.repository.Close()
}

// Path 返回数据库文件路径。
func (store *NMPAStore) Path() string {
	if store == nil || store.repository == nil {
		return ""
	}
	return store.repository.Path
}

// Size 返回数据库文件大小（含 WAL/SHM 由调用方决定是否统计）。
func (store *NMPAStore) Size() int64 {
	info, err := os.Stat(store.Path())
	if err != nil {
		return 0
	}
	return info.Size()
}

// Metadata 读取同步元数据。
func (store *NMPAStore) Metadata() (map[string]string, error) {
	rows, err := store.repository.db.Query(`SELECT key, value FROM nmpa_meta`)
	if err != nil {
		return nil, fmt.Errorf("读取药监同步元数据失败: %w", err)
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

// SetMetadata 写入同步元数据（不改变设备表）。
func (store *NMPAStore) SetMetadata(values map[string]string) error {
	tx, err := store.repository.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setMetadata(tx, values); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceAll 用一次全量结果替换整个参考库；失败时旧数据保持原样。
func (store *NMPAStore) ReplaceAll(devices []nmpa.Device, values map[string]string) error {
	tx, err := store.repository.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"nmpa_contacts", "nmpa_packages", "nmpa_devices"} {
		if _, err := tx.Exec(`DELETE FROM ` + quoteIdentifier(table)); err != nil {
			return fmt.Errorf("清空药监参考库失败: %w", err)
		}
	}
	if err := insertDevices(tx, devices, false); err != nil {
		return err
	}
	if err := setMetadata(tx, values); err != nil {
		return err
	}
	return tx.Commit()
}

// Upsert 写入一批周更设备；同一 Primary DI 覆盖旧值，并替换其包装/联系人。
func (store *NMPAStore) Upsert(devices []nmpa.Device, values map[string]string) error {
	tx, err := store.repository.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertDevices(tx, devices, true); err != nil {
		return err
	}
	if err := setMetadata(tx, values); err != nil {
		return err
	}
	return tx.Commit()
}

func insertDevices(tx *sql.Tx, devices []nmpa.Device, upsert bool) error {
	if len(devices) == 0 {
		return nil
	}
	placeholders := make([]string, len(nmpa.DeviceColumns)+3)
	for index := range placeholders {
		placeholders[index] = "?"
	}
	columns := append([]string{}, nmpa.DeviceColumns...)
	columns = append(columns, "package_count", "contact_count", "updated_at")
	query := `INSERT INTO nmpa_devices (` + strings.Join(quoteIdentifiers(columns), ",") + `) VALUES (` + strings.Join(placeholders, ",") + `)`
	if upsert {
		updates := make([]string, 0, len(columns)-1)
		for _, column := range columns[1:] {
			updates = append(updates, quoteIdentifier(column)+"=excluded."+quoteIdentifier(column))
		}
		query += ` ON CONFLICT(primary_di) DO UPDATE SET ` + strings.Join(updates, ",")
	}
	statement, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("准备药监设备写入失败: %w", err)
	}
	defer statement.Close()
	for _, device := range devices {
		if _, err := statement.Exec(deviceValues(device, nmpa.NowStamp(time.Now()))...); err != nil {
			return fmt.Errorf("写入药监设备失败: %w", err)
		}
		if err := replaceChildren(tx, device); err != nil {
			return err
		}
	}
	return nil
}

func replaceChildren(tx *sql.Tx, device nmpa.Device) error {
	if _, err := tx.Exec(`DELETE FROM nmpa_packages WHERE primary_di = ?`, device.PrimaryDI); err != nil {
		return fmt.Errorf("清理药监包装失败: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM nmpa_contacts WHERE primary_di = ?`, device.PrimaryDI); err != nil {
		return fmt.Errorf("清理药监联系人失败: %w", err)
	}
	for _, packing := range device.Packages {
		if _, err := tx.Exec(`INSERT INTO nmpa_packages (primary_di, package_di, package_type, lower_level_package_quantity, lower_level_package_identifier) VALUES (?, ?, ?, ?, ?)`,
			device.PrimaryDI, packing.PackageDI, packing.PackageType, packing.LowerLevelPackageQuantity, packing.LowerLevelPackageIdentifier); err != nil {
			return fmt.Errorf("写入药监包装失败: %w", err)
		}
	}
	for _, contact := range device.Contacts {
		if _, err := tx.Exec(`INSERT INTO nmpa_contacts (primary_di, fax, email, phone) VALUES (?, ?, ?, ?)`,
			device.PrimaryDI, contact.Fax, contact.Email, contact.Phone); err != nil {
			return fmt.Errorf("写入药监联系人失败: %w", err)
		}
	}
	return nil
}

func deviceValues(device nmpa.Device, stamp string) []any {
	return []any{
		device.PrimaryDI, device.IssuingAgency, device.PublishDate, device.QuantityPerMinSalesUnit,
		device.UnitOfUseDI, device.DataCarrier, device.IfConsistentWithRegistrationDI,
		device.RegistrationFilingDI, device.IfDirectMarking, device.IfDMIdenticalToMinSalesUnitDI,
		device.DirectMarking, device.DeviceGenericName, device.ProductName, device.Specification,
		device.IfKitProduct, device.ProductDescription, device.CatalogueCode,
		device.MedicalInsuranceCategoryCode, device.DeviceCategory, device.ClassificationCode,
		device.UnifiedSocialCreditCode, device.RegistrationFilingCertificateNo, device.RegistrantName,
		device.RegistrantEnglishName, device.MedicalInsuranceCode, device.ProductCategory,
		device.MRISafetyInformation, device.IfOneTimeUse, device.MinStorageDuration,
		device.IfSterilized, device.IfRequiresSterilizationBeforeUse, device.SterilizationMethod,
		device.OtherInfoURL, device.MarketExitDate, device.IfPIContainsLotNumber,
		device.IfPIContainsSerialNumber, device.IfPIContainsManufacturingDate,
		device.IfPIContainsExpiryDate, device.SpecialStorageHandlingType,
		device.SpecialStorageHandlingDescription, device.DeviceRecordKey, device.VersionNumber,
		device.VersionTime, device.VersionStatus, device.CorrectionNumber, device.CorrectionRemark,
		device.CorrectionTime, device.PackageCount, device.ContactCount, stamp,
	}
}

func setMetadata(tx *sql.Tx, values map[string]string) error {
	for key, value := range values {
		if _, err := tx.Exec(`INSERT INTO nmpa_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return fmt.Errorf("写入药监同步元数据失败: %w", err)
		}
	}
	return nil
}

// Counts 返回设备/包装/联系人行数。
func (store *NMPAStore) Counts() (int, int, int, error) {
	var devices, packages, contacts int
	if err := store.repository.db.QueryRow(`SELECT count(*) FROM nmpa_devices`).Scan(&devices); err != nil {
		return 0, 0, 0, err
	}
	if err := store.repository.db.QueryRow(`SELECT count(*) FROM nmpa_packages`).Scan(&packages); err != nil {
		return 0, 0, 0, err
	}
	if err := store.repository.db.QueryRow(`SELECT count(*) FROM nmpa_contacts`).Scan(&contacts); err != nil {
		return 0, 0, 0, err
	}
	return devices, packages, contacts, nil
}

// Search 按关键词查询设备主表；空关键词返回全部（按产品名、Primary DI 排序）。
func (store *NMPAStore) Search(query string, limit, offset int) (nmpa.QueryResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	pattern := "%" + strings.TrimSpace(query) + "%"
	where := `WHERE (? = '%%' OR primary_di LIKE ? OR product_name LIKE ? OR device_generic_name LIKE ? OR specification LIKE ? OR registrant_name LIKE ? OR registration_filing_certificate_number LIKE ?)`
	args := []any{pattern, pattern, pattern, pattern, pattern, pattern, pattern}
	var total int
	if err := store.repository.db.QueryRow(`SELECT count(*) FROM nmpa_devices `+where, args...).Scan(&total); err != nil {
		return nmpa.QueryResult{}, fmt.Errorf("统计药监查询结果失败: %w", err)
	}
	columns := append([]string{}, nmpa.DeviceColumns...)
	columns = append(columns, "package_count", "contact_count")
	querySQL := `SELECT ` + strings.Join(quoteIdentifiers(columns), ",") + ` FROM nmpa_devices ` + where + ` ORDER BY product_name, primary_di LIMIT ? OFFSET ?`
	rows, err := store.repository.db.Query(querySQL, append(args, limit, offset)...)
	if err != nil {
		return nmpa.QueryResult{}, fmt.Errorf("查询药监参考库失败: %w", err)
	}
	defer rows.Close()
	result := nmpa.QueryResult{Total: total}
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nmpa.QueryResult{}, err
		}
		result.Devices = append(result.Devices, device)
	}
	return result, rows.Err()
}

func scanDevice(rows *sql.Rows) (nmpa.Device, error) {
	var device nmpa.Device
	destinations := []any{
		&device.PrimaryDI, &device.IssuingAgency, &device.PublishDate, &device.QuantityPerMinSalesUnit,
		&device.UnitOfUseDI, &device.DataCarrier, &device.IfConsistentWithRegistrationDI,
		&device.RegistrationFilingDI, &device.IfDirectMarking, &device.IfDMIdenticalToMinSalesUnitDI,
		&device.DirectMarking, &device.DeviceGenericName, &device.ProductName, &device.Specification,
		&device.IfKitProduct, &device.ProductDescription, &device.CatalogueCode,
		&device.MedicalInsuranceCategoryCode, &device.DeviceCategory, &device.ClassificationCode,
		&device.UnifiedSocialCreditCode, &device.RegistrationFilingCertificateNo, &device.RegistrantName,
		&device.RegistrantEnglishName, &device.MedicalInsuranceCode, &device.ProductCategory,
		&device.MRISafetyInformation, &device.IfOneTimeUse, &device.MinStorageDuration,
		&device.IfSterilized, &device.IfRequiresSterilizationBeforeUse, &device.SterilizationMethod,
		&device.OtherInfoURL, &device.MarketExitDate, &device.IfPIContainsLotNumber,
		&device.IfPIContainsSerialNumber, &device.IfPIContainsManufacturingDate,
		&device.IfPIContainsExpiryDate, &device.SpecialStorageHandlingType,
		&device.SpecialStorageHandlingDescription, &device.DeviceRecordKey, &device.VersionNumber,
		&device.VersionTime, &device.VersionStatus, &device.CorrectionNumber, &device.CorrectionRemark,
		&device.CorrectionTime, &device.PackageCount, &device.ContactCount,
	}
	if err := rows.Scan(destinations...); err != nil {
		return nmpa.Device{}, fmt.Errorf("读取药监设备失败: %w", err)
	}
	return device, nil
}

func quoteIdentifiers(columns []string) []string {
	quoted := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = quoteIdentifier(column)
	}
	return quoted
}
