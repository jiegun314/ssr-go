package store

// AuditTable 是审计表名。审计与 operation_log 分开：
// operation_log 存"导出出去的记录"，audit_log 存"谁在什么身份下做了什么"。
const AuditTable = "audit_log"

var auditColumns = []string{
	"at", "actor", "role", "username", "machine", "action", "target", "detail", "result",
}

// AuditRecord 是 audit_log 的一行。
type AuditRecord struct {
	At       string // YYYY/MM/DD HH:MM:SS
	Actor    string // 登录时选的人（未登录为「未登录」）
	Role     string // ra | eama | udi | supply_chain | admin
	Username string // 本机操作系统用户名（只记录，不校验）
	Machine  string
	Action   string // login / logout / import / publish / consolidate / export / clear / settings / deny ...
	Target   string // 来源键、文件、表名等
	Detail   string // 简短说明（版本号/行数/原因）
	Result   string // ok | failed | denied
}

// AppendAudit 追加一条审计（表不存在时自动建表）。
func (repository *Repository) AppendAudit(record AuditRecord) error {
	row := Row{
		"at":       record.At,
		"actor":    record.Actor,
		"role":     record.Role,
		"username": record.Username,
		"machine":  record.Machine,
		"action":   record.Action,
		"target":   record.Target,
		"detail":   record.Detail,
		"result":   record.Result,
	}
	return repository.WriteRows(AuditTable, auditColumns, []Row{row}, WriteAppend)
}

// AuditRows 读全部审计（按写入顺序；界面/测试用）。
func (repository *Repository) AuditRows() ([]Row, error) {
	return repository.Rows(AuditTable)
}
