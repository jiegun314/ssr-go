// Package config 是四份 YAML 的唯一入口与校验点（R24）。
//
// 设计原则与 Python 版一致：**代码执行，配置声明**。表名、字段名、必填规则、匹配规则、
// 去重字段、变更描述、日志列全部来自配置；配置写错时程序不启动，绝不静默退化成默认行为。
//
// 移植自 core/config_loader.py。这里刻意不把 YAML 解成强类型结构体：Python 版是按
// dict 逐键校验的，强类型结构体会让「值类型不对」变成 YAML 解析失败，错误文案与校验
// 顺序都会和现状不一致（tests/test_config_loader.py 逐条断言了这些文案）。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jiegun314/ssr-go/internal/buildinfo"
	"github.com/jiegun314/ssr-go/internal/paths"
)

// 配置文件名。
const (
	SettingFile              = "setting.yaml"
	ImportMappingFile        = "excel_import_mapping.yaml"
	ConsolidationMappingFile = "consolidation_mapping.yaml"
	// LogColumnsFile 由 scripts/build_log_columns.py 从导出模板表头生成。
	LogColumnsFile = "log_columns.yaml"
)

// 日志记录自己的列：导出模板里没有它们，但表缺了它们就存不下应用写的东西。
const (
	LogStatusColumn = "status"
)

// LogOperationColumns 是应用级操作写入的两列。
var LogOperationColumns = []string{"operation", "details"}

// TransformTypes 是字段循环认识的 transform 类型。
var TransformTypes = []string{"direct", "first_not_null", "concatenate"}

// ChangeDescriptionTransform 是由变更比较派生出来的列（不是从来源表读的）。
const ChangeDescriptionTransform = "change_description"

// NoMatchActions 是来源没有基准记录时的四种动作（R13）。
var NoMatchActions = []string{"emit_incomplete", "empty", "skip", "error"}

// ChangeIdentityReference 是唯一被接受的比较身份写法。
const ChangeIdentityReference = "duplicate_check"

// ChangeDescriptionModes 是自动生成描述 vs 沿用来源文本。
var ChangeDescriptionModes = []string{"auto", "source"}

// MissingDescriptionActions 是 source 模式下来源没填描述时的两种处理。
var MissingDescriptionActions = []string{"error", "empty"}

// ChangeDescriptionPlaceholders 是 auto 模板允许使用的占位符。
var ChangeDescriptionPlaceholders = []string{"column", "chinese_name"}

// CleanupOnStartupEnv 可以在一次运行里覆盖 startup.cleanup_on_startup（R9）。
const CleanupOnStartupEnv = "SSR_CLEANUP_ON_STARTUP"

// Error 说明配置违反了它的契约；程序启动时直接失败，不带着错误配置运行。
type Error struct {
	Message string
}

func (e *Error) Error() string { return e.Message }

func errorf(format string, arguments ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, arguments...)}
}

// Loader 从一个配置目录加载全部 YAML。
type Loader struct {
	// Resolver 提供「相对 project_root」的路径策略（AGENTS.md §4.3）。
	Resolver paths.Resolver
	// Bootstrapped 记录本次补齐过的文件（配置文件缺失时从 defaults/ 复制过来的），
	// 交给启动日志说明情况。
	Bootstrapped []string
}

// DefaultsDirectory 是发布包里默认配置所在子目录：config/defaults/*.yaml。
// 发布包只带默认文件，正式名文件由第一次运行按需生成 —— 这样解压覆盖升级
// 永远不会覆盖用户改过的配置。
const DefaultsDirectory = "defaults"

// ConfigFileNames 是四份 YAML 的固定名字（顺序即启动校验顺序）。
var ConfigFileNames = []string{
	SettingFile, ImportMappingFile, ConsolidationMappingFile, LogColumnsFile,
}

// NewLoader 用与 Python 版相同的规则解析配置目录：显式参数 > UDI_CONFIG_DIR >
// 可执行文件同级的 config/。
//
// 解析完成后会先补齐缺失的配置文件（见 ensureConfigFiles）：只补"不存在"的，
// 存在但读不懂的一律留着让 ValidateAll 报错，绝不覆盖用户改过的内容。
func NewLoader(explicitConfigDir string) (*Loader, error) {
	resolver, err := paths.New(explicitConfigDir)
	if err != nil {
		return nil, err
	}
	loader := &Loader{Resolver: resolver}
	bootstrapped, err := loader.ensureConfigFiles()
	if err != nil {
		return nil, err
	}
	loader.Bootstrapped = bootstrapped
	return loader, nil
}

// ensureConfigFiles 把缺失的配置文件从默认文件复制出来，返回补齐的文件名列表。
//
// 判据只有一条：**文件不存在**。存在但解析/校验失败的文件不动 —— 那种情况要按
// R24 报错并拒绝启动，而不是悄悄用默认值覆盖用户改过的内容。
// 默认文件按两级找：配置目录里的 defaults/ → 可执行文件同级 config/defaults/
// （UDI_CONFIG_DIR 指到别处时，默认文件仍来自程序自带的这一份）。
func (loader *Loader) ensureConfigFiles() ([]string, error) {
	missing := make([]string, 0, len(ConfigFileNames))
	for _, name := range ConfigFileNames {
		if _, err := os.Stat(loader.Resolver.ConfigFile(name)); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, errorf("Configuration file not readable: %s: %v", loader.Resolver.ConfigFile(name), err)
			}
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	defaultsDirectory, ok := loader.defaultsDirectory()
	if !ok {
		// 没有默认文件可补：保持原来的"缺文件就报错"行为，由 LoadYAML 给出准确文案。
		return nil, nil
	}
	bootstrapped := make([]string, 0, len(missing))
	for _, name := range missing {
		source := filepath.Join(defaultsDirectory, name)
		content, err := os.ReadFile(source)
		if err != nil {
			// 默认文件本身缺了：同样交给 LoadYAML 报"配置找不到"。
			continue
		}
		target := loader.Resolver.ConfigFile(name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, errorf("Failed to create configuration folder: %v", err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return nil, errorf("Failed to write configuration file %s: %v", target, err)
		}
		bootstrapped = append(bootstrapped, name)
	}
	return bootstrapped, nil
}

// defaultsDirectory 找默认配置目录：配置目录里的 defaults/ 优先，
// 其次看程序自带的那一份（可执行文件同级的 config/defaults/）。
func (loader *Loader) defaultsDirectory() (string, bool) {
	candidates := []string{filepath.Join(loader.Resolver.ConfigDir, DefaultsDirectory)}
	if base := paths.BaseDirectory(); base != "" && base != loader.Resolver.ProjectRoot {
		candidates = append(candidates, filepath.Join(base, "config", DefaultsDirectory))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// ConfigDir 是四份 YAML 所在目录。
func (loader *Loader) ConfigDir() string { return loader.Resolver.ConfigDir }

// LoadYAML 读一份 YAML 并要求它的根是一个映射。
func (loader *Loader) LoadYAML(fileName string) (map[string]any, error) {
	path := loader.Resolver.ConfigFile(fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errorf("Configuration file not found: %s", path)
		}
		return nil, errorf("Configuration file not readable: %s: %v", path, err)
	}
	var decoded any
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		return nil, errorf("Invalid YAML in %s: %v", path, err)
	}
	document, ok := decoded.(map[string]any)
	if !ok {
		return nil, errorf("Configuration root must be a mapping: %s", path)
	}
	return document, nil
}

// LoadSetting 读 config/setting.yaml。
func (loader *Loader) LoadSetting() (map[string]any, error) {
	return loader.LoadYAML(SettingFile)
}

// LoadImportMapping 读 config/excel_import_mapping.yaml。
func (loader *Loader) LoadImportMapping() (map[string]any, error) {
	return loader.LoadYAML(ImportMappingFile)
}

// LoadConsolidationMapping 读 config/consolidation_mapping.yaml。
func (loader *Loader) LoadConsolidationMapping() (map[string]any, error) {
	return loader.LoadYAML(ConsolidationMappingFile)
}

// LoadLogColumnsDefinition 读 config/log_columns.yaml 的分组原样。
func (loader *Loader) LoadLogColumnsDefinition() (map[string]any, error) {
	return loader.LoadYAML(LogColumnsFile)
}

// LogColumns 把 before + columns + after 合成日志记录的列顺序（R22）。
// 顺序即写入顺序、查询顺序与日志回顾的显示顺序。
func LogColumns(definition map[string]any) []string {
	columns := []string{}
	for _, group := range []string{"before", "columns", "after"} {
		entries, ok := definition[group].([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			if text, ok := entry.(string); ok {
				columns = append(columns, text)
			}
		}
	}
	return columns
}

// LoadLogColumns 返回日志记录的列顺序。
func (loader *Loader) LoadLogColumns() ([]string, error) {
	definition, err := loader.LoadLogColumnsDefinition()
	if err != nil {
		return nil, err
	}
	return LogColumns(definition), nil
}

// LoadCleanupOnStartup 决定启动时是否清空暂存表（R9）。
//
// 环境变量优先于配置文件：发布包保持 cleanup_on_startup: true，让每次会话都从空的暂存表
// 开始；开发者想保留上一次导入的数据时，用 SSR_CLEANUP_ON_STARTUP 覆盖这一次运行，
// 而不是去改被跟踪的配置文件。取值写错直接报配置错误，不静默取默认值。
func (loader *Loader) LoadCleanupOnStartup() (bool, error) {
	if override, present := os.LookupEnv(CleanupOnStartupEnv); present && strings.TrimSpace(override) != "" {
		return ParseBoolean(override, CleanupOnStartupEnv)
	}
	setting, err := loader.LoadSetting()
	if err != nil {
		return false, err
	}
	startup, _ := setting["startup"].(map[string]any)
	if startup == nil {
		return true, nil
	}
	configured, present := startup["cleanup_on_startup"]
	if !present {
		return true, nil
	}
	value, ok := configured.(bool)
	if !ok {
		return true, nil
	}
	return value, nil
}

// ParseBoolean 解析 true/false、1/0、yes/no、on/off（大小写不敏感）。
func ParseBoolean(value string, source string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errorf("%s must be a boolean value, got: %q", source, value)
	}
}

// LoadDuplicateCheckIdentityFields 返回判断「这条结果是否已经存在」的列（R19）。
func (loader *Loader) LoadDuplicateCheckIdentityFields() ([]string, error) {
	consolidation, err := loader.LoadConsolidationMapping()
	if err != nil {
		return nil, err
	}
	dataset, err := requireMapping(consolidation["target_dataset"], "consolidation.target_dataset")
	if err != nil {
		return nil, err
	}
	duplicateCheck, _ := dataset["duplicate_check"].(map[string]any)
	fields, _ := duplicateCheck["identity_fields"].([]any)
	return stringList(fields), nil
}

// ChangeDescriptionRule 是派生列的比较规则（R18）。
type ChangeDescriptionRule struct {
	Field          string
	Fields         []string
	Labels         map[string]string
	IdentityFields []string
	CompareFields  any
	IgnoreFields   []string
	OnNewRecord    string
	OnNoChange     string
	OnOtherChange  map[string]any
}

// LoadChangeDescriptionRule 解析由变更比较派生的导出列；没有该列时返回 nil。
//
// 身份字段直接引用 duplicate_check.identity_fields，所以重复判定与变更比较不会各维护
// 一份字段列表而漂移。
func (loader *Loader) LoadChangeDescriptionRule() (*ChangeDescriptionRule, error) {
	consolidation, err := loader.LoadConsolidationMapping()
	if err != nil {
		return nil, err
	}
	dataset, err := requireMapping(consolidation["target_dataset"], "consolidation.target_dataset")
	if err != nil {
		return nil, err
	}
	fields, _ := dataset["fields"].(map[string]any)
	fieldNames := mappingKeys(fields)
	for _, fieldName := range fieldNames {
		definition, _ := fields[fieldName].(map[string]any)
		transform, _ := definition["transform"].(map[string]any)
		transformType, _ := transform["type"].(string)
		if transformType != ChangeDescriptionTransform {
			continue
		}
		identityFields, err := loader.LoadDuplicateCheckIdentityFields()
		if err != nil {
			return nil, err
		}
		labels := map[string]string{}
		for name, raw := range fields {
			item, _ := raw.(map[string]any)
			label, _ := item["chinese_name"].(string)
			if label == "" {
				label = name
			}
			labels[name] = label
		}
		compareFields, present := transform["compare_fields"]
		if !present {
			compareFields = "all"
		}
		onNewRecord, _ := transform["on_new_record"].(string)
		onNoChange, _ := transform["on_no_change"].(string)
		onOtherChange, _ := transform["on_other_change"].(map[string]any)
		return &ChangeDescriptionRule{
			Field:          fieldName,
			Fields:         fieldNames,
			Labels:         labels,
			IdentityFields: identityFields,
			CompareFields:  compareFields,
			IgnoreFields:   stringList(transform["ignore_fields"]),
			OnNewRecord:    onNewRecord,
			OnNoChange:     onNoChange,
			OnOtherChange:  onOtherChange,
		}, nil
	}
	return nil, nil
}

// ResolvePath 把配置里的路径解析成绝对路径（相对配置目录的父目录）。
func (loader *Loader) ResolvePath(configuredPath string) string {
	return loader.Resolver.Resolve(configuredPath)
}

// ValidateAll 在启动时整体校验四份配置（R24）：任何一处不合法都让程序拒绝启动。
//
// 校验顺序与 Python 版一致（setting → import → consolidation → log columns），
// 因此「先报哪个错」也是一致的。
func (loader *Loader) ValidateAll() error {
	setting, err := loader.LoadSetting()
	if err != nil {
		return err
	}
	imports, err := loader.LoadImportMapping()
	if err != nil {
		return err
	}
	consolidation, err := loader.LoadConsolidationMapping()
	if err != nil {
		return err
	}
	if err := loader.ValidateSetting(setting); err != nil {
		return err
	}
	if err := loader.ValidateImportMapping(imports); err != nil {
		return err
	}
	if err := loader.ValidateConsolidationMapping(consolidation, imports, setting); err != nil {
		return err
	}
	definition, err := loader.LoadLogColumnsDefinition()
	if err != nil {
		return err
	}
	return loader.ValidateLogColumns(definition, setting, consolidation)
}

// --- 取值与断言的小工具（对应 Python 版的 dict.get / isinstance 组合） ---

func requireMapping(value any, path string) (map[string]any, error) {
	if mapping, ok := value.(map[string]any); ok {
		return mapping, nil
	}
	return nil, errorf("%s must be a mapping", path)
}

func asString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

func stringList(value any) []string {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		if text, ok := entry.(string); ok {
			results = append(results, text)
		}
	}
	return results
}

// stringListStrict 要求列表里的每一项都是字符串（配置校验用）。
func stringListStrict(value any) ([]string, bool) {
	entries, ok := value.([]any)
	if !ok {
		return nil, false
	}
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		text, ok := entry.(string)
		if !ok {
			return nil, false
		}
		results = append(results, text)
	}
	return results, true
}

func mappingKeys(mapping map[string]any) []string {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	return keys
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// IsVersionString 让本包不必重复引入 buildinfo 的判定规则。
func IsVersionString(value any) bool { return buildinfo.IsVersionString(value) }
