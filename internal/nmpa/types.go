// Package nmpa 是药监 UDI 公共数据的本地参考库。
//
// 它只做三件事：读 RSS 发布索引、下载发布包、按 SSR 配置的统一社会信用代码
// 过滤后写入独立 SQLite。药监数据不进入导入/整合流程，也不写 global_udi_input。
package nmpa

import "time"

// ReleaseKind 是药监发布文件的频率类型。
type ReleaseKind string

const (
	KindFull    ReleaseKind = "full"
	KindMonthly ReleaseKind = "monthly"
	KindWeekly  ReleaseKind = "weekly"
	KindDaily   ReleaseKind = "daily"
)

// Release 是 RSS 里的一个发布包。
type Release struct {
	Kind        ReleaseKind `json:"kind"`
	FileName    string      `json:"fileName"`
	Title       string      `json:"title"`
	URL         string      `json:"url"`
	PublishedAt string      `json:"publishedAt"`
	// Date 是发布包自身的日期：全量/日更为 YYYY-MM-DD；
	// 月更为 YYYY-MM；周更为结束日 YYYY-MM-DD。
	Date  string `json:"date"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	Count int    `json:"count,omitempty"`
}

// Status 是界面看到的同步状态。
type Status struct {
	SocialCreditCode string `json:"socialCreditCode"`
	CodeConfigured   bool   `json:"codeConfigured"`

	FullRelease *Release `json:"fullRelease,omitempty"`
	// WeeklyReleases 是 RSS 当前可见的周更（旧的在前）。
	WeeklyReleases []Release `json:"weeklyReleases,omitempty"`

	LastFullFile    string `json:"lastFullFile"`
	LastFullDate    string `json:"lastFullDate"`
	LastFullAt      string `json:"lastFullAt"`
	LastWeekEnd     string `json:"lastWeekEnd"`
	LastWeeklyFile  string `json:"lastWeeklyFile"`
	LastSyncAt      string `json:"lastSyncAt"`
	LastCheckAt     string `json:"lastCheckAt"`
	LastError       string `json:"lastError"`
	DeviceCount     int    `json:"deviceCount"`
	PackageCount    int    `json:"packageCount"`
	ContactCount    int    `json:"contactCount"`
	DatabasePath    string `json:"databasePath"`
	DatabaseBytes   int64  `json:"databaseBytes"`
	NeedsFull       bool   `json:"needsFull"`
	CodeChanged     bool   `json:"codeChanged"`
	WeeklyAvailable bool   `json:"weeklyAvailable"`
	FullDue         bool   `json:"fullDue"`
}

// SyncResult 是一次全量/增量同步的结果。
type SyncResult struct {
	Mode     string `json:"mode"`
	FileName string `json:"fileName"`
	Matched  int    `json:"matched"`
	Devices  int    `json:"devices"`
	Packages int    `json:"packages"`
	Contacts int    `json:"contacts"`
	Message  string `json:"message"`
	Status   Status `json:"status"`
}

// QueryResult 是本地参考库的查询结果。
type QueryResult struct {
	Total   int      `json:"total"`
	Devices []Device `json:"devices"`
}

// Device 是标准设备表的一行。字段名对应 NMPA XML 的中文拼音字段，
// 但落库/查询一律用可读英文名，原字段映射见 README/字段表。
type Device struct {
	PrimaryDI                         string `json:"primaryDi"`
	IssuingAgency                     string `json:"issuingAgency"`
	PublishDate                       string `json:"publishDate"`
	QuantityPerMinSalesUnit           string `json:"quantityPerMinSalesUnit"`
	UnitOfUseDI                       string `json:"unitOfUseDi"`
	DataCarrier                       string `json:"dataCarrier"`
	IfConsistentWithRegistrationDI    string `json:"ifConsistentWithRegistrationDi"`
	RegistrationFilingDI              string `json:"registrationFilingDi"`
	IfDirectMarking                   string `json:"ifDirectMarking"`
	IfDMIdenticalToMinSalesUnitDI     string `json:"ifDmIdenticalToMinSalesUnitDi"`
	DirectMarking                     string `json:"directMarking"`
	DeviceGenericName                 string `json:"deviceGenericName"`
	ProductName                       string `json:"productName"`
	Specification                     string `json:"specification"`
	IfKitProduct                      string `json:"ifKitProduct"`
	ProductDescription                string `json:"productDescription"`
	CatalogueCode                     string `json:"catalogueCode"`
	MedicalInsuranceCategoryCode      string `json:"medicalInsuranceCategoryCode"`
	DeviceCategory                    string `json:"deviceCategory"`
	ClassificationCode                string `json:"classificationCode"`
	UnifiedSocialCreditCode           string `json:"unifiedSocialCreditCode"`
	RegistrationFilingCertificateNo   string `json:"registrationFilingCertificateNumber"`
	RegistrantName                    string `json:"registrantName"`
	RegistrantEnglishName             string `json:"registrantEnglishName"`
	MedicalInsuranceCode              string `json:"medicalInsuranceCode"`
	ProductCategory                   string `json:"productCategory"`
	MRISafetyInformation              string `json:"mriSafetyInformation"`
	IfOneTimeUse                      string `json:"ifOneTimeUse"`
	MinStorageDuration                string `json:"minStorageDuration"`
	IfSterilized                      string `json:"ifSterilized"`
	IfRequiresSterilizationBeforeUse  string `json:"ifRequiresSterilizationBeforeUse"`
	SterilizationMethod               string `json:"sterilizationMethod"`
	OtherInfoURL                      string `json:"otherInfoUrl"`
	MarketExitDate                    string `json:"marketExitDate"`
	IfPIContainsLotNumber             string `json:"ifPiContainsLotNumber"`
	IfPIContainsSerialNumber          string `json:"ifPiContainsSerialNumber"`
	IfPIContainsManufacturingDate     string `json:"ifPiContainsManufacturingDate"`
	IfPIContainsExpiryDate            string `json:"ifPiContainsExpiryDate"`
	SpecialStorageHandlingType        string `json:"specialStorageHandlingType"`
	SpecialStorageHandlingDescription string `json:"specialStorageHandlingDescription"`
	DeviceRecordKey                   string `json:"deviceRecordKey"`
	VersionNumber                     string `json:"versionNumber"`
	VersionTime                       string `json:"versionTime"`
	VersionStatus                     string `json:"versionStatus"`
	CorrectionNumber                  string `json:"correctionNumber"`
	CorrectionRemark                  string `json:"correctionRemark"`
	CorrectionTime                    string `json:"correctionTime"`

	PackageCount int       `json:"packageCount"`
	ContactCount int       `json:"contactCount"`
	Packages     []Package `json:"packages,omitempty"`
	Contacts     []Contact `json:"contacts,omitempty"`
}

// Package 是包装层级。
type Package struct {
	PackageDI                   string `json:"packageDi"`
	PackageType                 string `json:"packageType"`
	LowerLevelPackageQuantity   string `json:"lowerLevelPackageQuantity"`
	LowerLevelPackageIdentifier string `json:"lowerLevelPackageIdentifier"`
}

// Contact 是企业联系人。
type Contact struct {
	Fax   string `json:"fax"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// DeviceColumns 是设备主表的列顺序。SQLite 建表与查询共用这一份。
var DeviceColumns = []string{
	"primary_di", "issuing_agency", "publish_date", "quantity_per_min_sales_unit",
	"unit_of_use_di", "data_carrier", "if_consistent_with_registration_di",
	"registration_filing_di", "if_direct_marking", "if_dm_identical_to_min_sales_unit_di",
	"direct_marking", "device_generic_name", "product_name", "specification",
	"if_kit_product", "product_description", "catalogue_code",
	"medical_insurance_category_code", "device_category", "classification_code",
	"unified_social_credit_code", "registration_filing_certificate_number",
	"registrant_name", "registrant_english_name", "medical_insurance_code",
	"product_category", "mri_safety_information", "if_one_time_use",
	"min_storage_duration", "if_sterilized", "if_requires_sterilization_before_use",
	"sterilization_method", "other_info_url", "market_exit_date",
	"if_pi_contains_lot_number", "if_pi_contains_serial_number",
	"if_pi_contains_manufacturing_date", "if_pi_contains_expiry_date",
	"special_storage_handling_type", "special_storage_handling_description",
	"device_record_key", "version_number", "version_time", "version_status",
	"correction_number", "correction_remark", "correction_time",
}

// DeviceColumnLabels 是字段的中文名，给查询结果表头使用。
var DeviceColumnLabels = map[string]string{
	"primary_di": "Primary DI", "issuing_agency": "发码机构", "publish_date": "发布日期",
	"quantity_per_min_sales_unit": "最小销售单元数量", "unit_of_use_di": "使用单元 DI",
	"data_carrier": "标识载体", "if_consistent_with_registration_di": "与注册 DI 一致",
	"registration_filing_di": "注册/备案 DI", "if_direct_marking": "有直接标记 DI",
	"if_dm_identical_to_min_sales_unit_di": "直接标记 DI 同主 DI", "direct_marking": "直接标记 DI",
	"device_generic_name": "通用名称", "product_name": "产品名称", "specification": "规格型号",
	"if_kit_product": "包类产品", "product_description": "产品描述", "catalogue_code": "目录编码",
	"medical_insurance_category_code": "医保分类编码", "device_category": "器械类别",
	"classification_code": "分类编码", "unified_social_credit_code": "统一社会信用代码",
	"registration_filing_certificate_number": "注册证/备案凭证编号",
	"registrant_name":                        "注册人/备案人", "registrant_english_name": "注册人英文名",
	"medical_insurance_code": "医保编码", "product_category": "产品类别",
	"mri_safety_information": "MRI 安全信息", "if_one_time_use": "一次性使用",
	"min_storage_duration": "最短储存期", "if_sterilized": "无菌包装",
	"if_requires_sterilization_before_use": "使用前需灭菌", "sterilization_method": "灭菌方式",
	"other_info_url": "其他信息网址", "market_exit_date": "退市日期",
	"if_pi_contains_lot_number": "PI 含批号", "if_pi_contains_serial_number": "PI 含序列号",
	"if_pi_contains_manufacturing_date": "PI 含生产日期", "if_pi_contains_expiry_date": "PI 含失效日期",
	"special_storage_handling_type":        "特殊储存处理类型",
	"special_storage_handling_description": "特殊储存处理说明",
	"device_record_key":                    "记录键", "version_number": "版本号", "version_time": "版本时间",
	"version_status": "版本状态", "correction_number": "更正次数",
	"correction_remark": "更正说明", "correction_time": "更正时间",
}

// NowStamp 是写元数据用的时间格式。
func NowStamp(now time.Time) string { return now.Format("2006-01-02 15:04:05") }
