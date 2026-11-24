package app

import (
	"fmt"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/nmpa"
)

// NMPAField 是详情抽屉中的一个只读字段。
type NMPAField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// NMPAFieldGroup 是详情抽屉中的一组字段。
type NMPAFieldGroup struct {
	Title  string      `json:"title"`
	Fields []NMPAField `json:"fields"`
}

// NMPAPackage 是详情/全字段导出中的包装层级。
type NMPAPackage struct {
	PackageDI                   string `json:"packageDi"`
	PackageType                 string `json:"packageType"`
	LowerLevelPackageQuantity   string `json:"lowerLevelPackageQuantity"`
	LowerLevelPackageIdentifier string `json:"lowerLevelPackageIdentifier"`
}

// NMPAContact 是详情/全字段导出中的企业联系人。
type NMPAContact struct {
	Fax   string `json:"fax"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// NMPADeviceDetail 是行详情抽屉的数据。主表字段按业务分组返回，避免前端维护字段清单。
type NMPADeviceDetail struct {
	PrimaryDI       string           `json:"primaryDi"`
	DisplayName     string           `json:"displayName"`
	VersionStatus   string           `json:"versionStatus"`
	ProductCategory string           `json:"productCategory"`
	Groups          []NMPAFieldGroup `json:"groups"`
	Packages        []NMPAPackage    `json:"packages"`
	Contacts        []NMPAContact    `json:"contacts"`
}

var nmpaFieldGroups = []struct {
	Title   string
	Columns []string
}{
	{"标识与包装", []string{"primary_di", "issuing_agency", "publish_date", "quantity_per_min_sales_unit", "unit_of_use_di", "data_carrier"}},
	{"注册与直接标记", []string{"if_consistent_with_registration_di", "registration_filing_di", "if_direct_marking", "if_dm_identical_to_min_sales_unit_di", "direct_marking", "registration_filing_certificate_number"}},
	{"产品信息", []string{"device_generic_name", "product_name", "specification", "if_kit_product", "product_description", "catalogue_code"}},
	{"注册人与企业", []string{"unified_social_credit_code", "registrant_name", "registrant_english_name"}},
	{"分类与医保", []string{"medical_insurance_category_code", "device_category", "classification_code", "medical_insurance_code", "product_category"}},
	{"使用与储存", []string{"mri_safety_information", "if_one_time_use", "min_storage_duration", "if_sterilized", "if_requires_sterilization_before_use", "sterilization_method", "other_info_url", "market_exit_date"}},
	{"PI 标识", []string{"if_pi_contains_lot_number", "if_pi_contains_serial_number", "if_pi_contains_manufacturing_date", "if_pi_contains_expiry_date"}},
	{"特殊储存", []string{"special_storage_handling_type", "special_storage_handling_description"}},
	{"版本与更正", []string{"device_record_key", "version_number", "version_time", "version_status", "correction_number", "correction_remark", "correction_time"}},
}

// nmpaDeviceValues 按 nmpa.DeviceColumns 的顺序返回 47 个标准字段。
func nmpaDeviceValues(device nmpa.Device) []string {
	return []string{
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
		device.IfPIContainsSerialNumber, device.IfPIContainsManufacturingDate, device.IfPIContainsExpiryDate,
		device.SpecialStorageHandlingType, device.SpecialStorageHandlingDescription,
		device.DeviceRecordKey, device.VersionNumber, device.VersionTime, device.VersionStatus,
		device.CorrectionNumber, device.CorrectionRemark, device.CorrectionTime,
	}
}

func nmpaDisplayName(device nmpa.Device) string {
	if name := device.ProductName; name != "" {
		return name
	}
	return device.DeviceGenericName
}

func nmpaAllColumns() []string {
	columns := make([]string, len(nmpa.DeviceColumns))
	for index, column := range nmpa.DeviceColumns {
		columns[index] = nmpa.DeviceColumnLabels[column]
	}
	return columns
}

func nmpaDeviceDetail(device nmpa.Device) NMPADeviceDetail {
	rawValues := nmpaDeviceValues(device)
	values := map[string]string{}
	for index, column := range nmpa.DeviceColumns {
		values[column] = rawValues[index]
	}
	detail := NMPADeviceDetail{
		PrimaryDI:       device.PrimaryDI,
		DisplayName:     nmpaDisplayName(device),
		VersionStatus:   device.VersionStatus,
		ProductCategory: device.ProductCategory,
	}
	for _, group := range nmpaFieldGroups {
		fields := make([]NMPAField, 0, len(group.Columns))
		for _, column := range group.Columns {
			fields = append(fields, NMPAField{Label: nmpa.DeviceColumnLabels[column], Value: values[column]})
		}
		detail.Groups = append(detail.Groups, NMPAFieldGroup{Title: group.Title, Fields: fields})
	}
	for _, packing := range device.Packages {
		detail.Packages = append(detail.Packages, NMPAPackage{
			PackageDI: packing.PackageDI, PackageType: packing.PackageType,
			LowerLevelPackageQuantity:   packing.LowerLevelPackageQuantity,
			LowerLevelPackageIdentifier: packing.LowerLevelPackageIdentifier,
		})
	}
	for _, contact := range device.Contacts {
		detail.Contacts = append(detail.Contacts, NMPAContact{Fax: contact.Fax, Email: contact.Email, Phone: contact.Phone})
	}
	return detail
}

// NMPADetail 返回单台设备的全部字段、包装和联系人（Admin / Supply Chain 只读）。
func (app *App) NMPADetail(primaryDI string) (NMPADeviceDetail, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.authorize(auth.ActionNMPAView); err != nil {
		return NMPADeviceDetail{}, err
	}
	service, err := app.nmpaServiceOrError()
	if err != nil {
		return NMPADeviceDetail{}, err
	}
	device, err := service.Device(primaryDI)
	if err != nil {
		return NMPADeviceDetail{}, fmt.Errorf("读取药监设备详情失败: %w", err)
	}
	return nmpaDeviceDetail(device), nil
}
