package nmpa

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// deviceXML 直接对应药监的 <device> 节点；字段用拼音名，避免和标准表字段混淆。
type deviceXML struct {
	PrimaryDI                         string       `xml:"zxxsdycpbs"`
	IssuingAgency                     string       `xml:"cpbsbmtxmc"`
	PublishDate                       string       `xml:"cpbsfbrq"`
	QuantityPerMinSalesUnit           string       `xml:"zxxsdyzsydydsl"`
	UnitOfUseDI                       string       `xml:"sydycpbs"`
	DataCarrier                       string       `xml:"bszt"`
	IfConsistentWithRegistrationDI    string       `xml:"sfyzcbayz"`
	RegistrationFilingDI              string       `xml:"zcbacpbs"`
	IfDirectMarking                   string       `xml:"sfybtzjbs"`
	IfDMIdenticalToMinSalesUnitDI     string       `xml:"btcpbsyzxxsdycpbssfyz"`
	DirectMarking                     string       `xml:"btcpbs"`
	DeviceGenericName                 string       `xml:"cpmctymc"`
	ProductName                       string       `xml:"spmc"`
	Specification                     string       `xml:"ggxh"`
	IfKitProduct                      string       `xml:"sfwblztlcp"`
	ProductDescription                string       `xml:"cpms"`
	CatalogueCode                     string       `xml:"cphhhbh"`
	MedicalInsuranceCategoryCode      string       `xml:"yflbm"`
	DeviceCategory                    string       `xml:"qxlb"`
	ClassificationCode                string       `xml:"flbm"`
	UnifiedSocialCreditCode           string       `xml:"tyshxydm"`
	RegistrationFilingCertificateNo   string       `xml:"zczbhhzbapzbh"`
	RegistrantName                    string       `xml:"ylqxzcrbarmc"`
	RegistrantEnglishName             string       `xml:"ylqxzcrbarywmc"`
	MedicalInsuranceCode              string       `xml:"ybbm"`
	ProductCategory                   string       `xml:"cplb"`
	MRISafetyInformation              string       `xml:"cgzmraqxgxx"`
	IfOneTimeUse                      string       `xml:"sfbjwycxsy"`
	MinStorageDuration                string       `xml:"zdcfsycs"`
	IfSterilized                      string       `xml:"sfwwjbz"`
	IfRequiresSterilizationBeforeUse  string       `xml:"syqsfxyjxmj"`
	SterilizationMethod               string       `xml:"mjfs"`
	OtherInfoURL                      string       `xml:"qtxxdwzlj"`
	MarketExitDate                    string       `xml:"tsrq"`
	IfPIContainsLotNumber             string       `xml:"scbssfbhph"`
	IfPIContainsSerialNumber          string       `xml:"scbssfbhxlh"`
	IfPIContainsManufacturingDate     string       `xml:"scbssfbhscrq"`
	IfPIContainsExpiryDate            string       `xml:"scbssfbhsxrq"`
	SpecialStorageHandlingType        string       `xml:"tscchcztj"`
	SpecialStorageHandlingDescription string       `xml:"tsccsm"`
	DeviceRecordKey                   string       `xml:"deviceRecordKey"`
	VersionNumber                     string       `xml:"versionNumber"`
	VersionTime                       string       `xml:"versionTime"`
	VersionStatus                     string       `xml:"versionStauts"`
	CorrectionNumber                  string       `xml:"correctionNumber"`
	CorrectionRemark                  string       `xml:"correctionRemark"`
	CorrectionTime                    string       `xml:"correctionTime"`
	Packings                          []packingXML `xml:"packingList>packing"`
	Contacts                          []contactXML `xml:"contactList>contact"`
}

type packingXML struct {
	PackageDI                   string `xml:"bzcpbs"`
	PackageType                 string `xml:"cpbzjb"`
	LowerLevelPackageQuantity   string `xml:"bznhxyjcpbssl"`
	LowerLevelPackageIdentifier string `xml:"bznhxyjbzcpbs"`
}

type contactXML struct {
	Fax   string `xml:"qylxrcz"`
	Email string `xml:"qylxryx"`
	Phone string `xml:"qylxrdh"`
}

// ParsedArchive 是从一个发布包里筛选出的数据。
type ParsedArchive struct {
	Devices []Device
	Skipped int
	Total   int
}

// ParseArchive 打开 zip，流式读取其中 XML，只保留统一社会信用代码匹配的设备。
//
// 不把整份 XML 解压到磁盘，也不把不匹配的设备放进内存。全量包很大，但这里
// 的峰值内存只与"匹配到的设备数"有关。
func ParseArchive(zipPath string, socialCreditCode string) (ParsedArchive, error) {
	expected := normalizeCode(socialCreditCode)
	if expected == "" {
		return ParsedArchive{}, fmt.Errorf("统一社会信用代码为空")
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return ParsedArchive{}, fmt.Errorf("打开 NMPA 压缩包失败: %w", err)
	}
	defer reader.Close()

	devices := map[string]Device{}
	total := 0
	skipped := 0
	foundXML := false
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".xml") {
			continue
		}
		foundXML = true
		if err := parseXMLFile(file, expected, devices, &total, &skipped); err != nil {
			return ParsedArchive{}, err
		}
	}
	if !foundXML {
		return ParsedArchive{}, fmt.Errorf("NMPA 压缩包中没有 XML 文件")
	}
	result := ParsedArchive{Total: total, Skipped: skipped, Devices: make([]Device, 0, len(devices))}
	for _, device := range devices {
		result.Devices = append(result.Devices, device)
	}
	sortDevices(result.Devices)
	return result, nil
}

func parseXMLFile(file *zip.File, expected string, devices map[string]Device, total, skipped *int) error {
	stream, err := file.Open()
	if err != nil {
		return fmt.Errorf("读取 NMPA XML 失败: %w", err)
	}
	defer stream.Close()
	decoder := xml.NewDecoder(&xmlControlSanitizer{reader: bufio.NewReaderSize(stream, 64*1024)})
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("解析 NMPA XML 失败: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "device" {
			continue
		}
		var raw deviceXML
		if err := decoder.DecodeElement(&raw, &start); err != nil {
			return fmt.Errorf("解析 NMPA 设备记录失败: %w", err)
		}
		*total++
		if normalizeCode(raw.UnifiedSocialCreditCode) != expected {
			*skipped++
			continue
		}
		device := mapDevice(raw)
		if device.PrimaryDI == "" {
			*skipped++
			continue
		}
		if existing, ok := devices[device.PrimaryDI]; ok && existing.VersionTime > device.VersionTime {
			continue
		}
		devices[device.PrimaryDI] = device
	}
}

func mapDevice(raw deviceXML) Device {
	device := Device{
		PrimaryDI:                         clean(raw.PrimaryDI),
		IssuingAgency:                     clean(raw.IssuingAgency),
		PublishDate:                       clean(raw.PublishDate),
		QuantityPerMinSalesUnit:           clean(raw.QuantityPerMinSalesUnit),
		UnitOfUseDI:                       clean(raw.UnitOfUseDI),
		DataCarrier:                       clean(raw.DataCarrier),
		IfConsistentWithRegistrationDI:    clean(raw.IfConsistentWithRegistrationDI),
		RegistrationFilingDI:              clean(raw.RegistrationFilingDI),
		IfDirectMarking:                   clean(raw.IfDirectMarking),
		IfDMIdenticalToMinSalesUnitDI:     clean(raw.IfDMIdenticalToMinSalesUnitDI),
		DirectMarking:                     clean(raw.DirectMarking),
		DeviceGenericName:                 clean(raw.DeviceGenericName),
		ProductName:                       clean(raw.ProductName),
		Specification:                     clean(raw.Specification),
		IfKitProduct:                      clean(raw.IfKitProduct),
		ProductDescription:                clean(raw.ProductDescription),
		CatalogueCode:                     clean(raw.CatalogueCode),
		MedicalInsuranceCategoryCode:      clean(raw.MedicalInsuranceCategoryCode),
		DeviceCategory:                    clean(raw.DeviceCategory),
		ClassificationCode:                clean(raw.ClassificationCode),
		UnifiedSocialCreditCode:           clean(raw.UnifiedSocialCreditCode),
		RegistrationFilingCertificateNo:   clean(raw.RegistrationFilingCertificateNo),
		RegistrantName:                    clean(raw.RegistrantName),
		RegistrantEnglishName:             clean(raw.RegistrantEnglishName),
		MedicalInsuranceCode:              clean(raw.MedicalInsuranceCode),
		ProductCategory:                   clean(raw.ProductCategory),
		MRISafetyInformation:              clean(raw.MRISafetyInformation),
		IfOneTimeUse:                      clean(raw.IfOneTimeUse),
		MinStorageDuration:                clean(raw.MinStorageDuration),
		IfSterilized:                      clean(raw.IfSterilized),
		IfRequiresSterilizationBeforeUse:  clean(raw.IfRequiresSterilizationBeforeUse),
		SterilizationMethod:               clean(raw.SterilizationMethod),
		OtherInfoURL:                      clean(raw.OtherInfoURL),
		MarketExitDate:                    clean(raw.MarketExitDate),
		IfPIContainsLotNumber:             clean(raw.IfPIContainsLotNumber),
		IfPIContainsSerialNumber:          clean(raw.IfPIContainsSerialNumber),
		IfPIContainsManufacturingDate:     clean(raw.IfPIContainsManufacturingDate),
		IfPIContainsExpiryDate:            clean(raw.IfPIContainsExpiryDate),
		SpecialStorageHandlingType:        clean(raw.SpecialStorageHandlingType),
		SpecialStorageHandlingDescription: clean(raw.SpecialStorageHandlingDescription),
		DeviceRecordKey:                   clean(raw.DeviceRecordKey),
		VersionNumber:                     clean(raw.VersionNumber),
		VersionTime:                       clean(raw.VersionTime),
		VersionStatus:                     clean(raw.VersionStatus),
		CorrectionNumber:                  clean(raw.CorrectionNumber),
		CorrectionRemark:                  clean(raw.CorrectionRemark),
		CorrectionTime:                    clean(raw.CorrectionTime),
	}
	if device.DeviceRecordKey == "" {
		device.DeviceRecordKey = device.PrimaryDI
	}
	for _, packing := range raw.Packings {
		device.Packages = append(device.Packages, Package{
			PackageDI:                   clean(packing.PackageDI),
			PackageType:                 clean(packing.PackageType),
			LowerLevelPackageQuantity:   clean(packing.LowerLevelPackageQuantity),
			LowerLevelPackageIdentifier: clean(packing.LowerLevelPackageIdentifier),
		})
	}
	for _, contact := range raw.Contacts {
		device.Contacts = append(device.Contacts, Contact{
			Fax:   clean(contact.Fax),
			Email: clean(contact.Email),
			Phone: clean(contact.Phone),
		})
	}
	device.PackageCount = len(device.Packages)
	device.ContactCount = len(device.Contacts)
	return device
}

func clean(value string) string {
	return strings.TrimSpace(strings.Trim(value, "\ufeff"))
}

func normalizeCode(value string) string {
	return strings.ToUpper(clean(value))
}

func sortDevices(devices []Device) {
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].ProductName == devices[j].ProductName {
			return devices[i].PrimaryDI < devices[j].PrimaryDI
		}
		return devices[i].ProductName < devices[j].ProductName
	})
}

// xmlControlSanitizer 丢弃 XML 1.0 不允许的 C0 控制字符。
//
// 药监全量包中确实出现过 U+0002 一类的坏字节；它们只可能以单字节形式
// 出现在 UTF-8 流里，因此按字节过滤不会破坏中文等多字节字符。
type xmlControlSanitizer struct {
	reader  *bufio.Reader
	pending []byte
}

func (sanitizer *xmlControlSanitizer) Read(buffer []byte) (int, error) {
	count := 0
	if len(sanitizer.pending) > 0 {
		count = copy(buffer, sanitizer.pending)
		sanitizer.pending = sanitizer.pending[count:]
	}
	for count < len(buffer) {
		value, size, err := sanitizer.reader.ReadRune()
		if err != nil {
			if count > 0 {
				return count, nil
			}
			return 0, err
		}
		if !validXMLRune(value) || (value == utf8.RuneError && size == 1) {
			continue
		}
		if count+size > len(buffer) {
			var encoded [utf8.UTFMax]byte
			length := utf8.EncodeRune(encoded[:], value)
			sanitizer.pending = append(sanitizer.pending, encoded[:length]...)
			return count, nil
		}
		count += utf8.EncodeRune(buffer[count:], value)
	}
	return count, nil
}

func validXMLRune(value rune) bool {
	if value == '\t' || value == '\n' || value == '\r' {
		return true
	}
	if value < 0x20 || value == 0xFFFE || value == 0xFFFF {
		return false
	}
	if value >= 0xD800 && value <= 0xDFFF {
		return false
	}
	return true
}
