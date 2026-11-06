export namespace app {
	
	export class AboutInfo {
	    name: string;
	    tagline: string;
	    chineseName: string;
	    version: string;
	    detail: string;
	    dataDir: string;
	    configDir: string;
	    database: string;
	    runtime: string;
	    techStack: string[];
	
	    static createFrom(source: any = {}) {
	        return new AboutInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.tagline = source["tagline"];
	        this.chineseName = source["chineseName"];
	        this.version = source["version"];
	        this.detail = source["detail"];
	        this.dataDir = source["dataDir"];
	        this.configDir = source["configDir"];
	        this.database = source["database"];
	        this.runtime = source["runtime"];
	        this.techStack = source["techStack"];
	    }
	}
	export class ImportState {
	    source: string;
	    state: string;
	    label: string;
	    tooltip: string;
	    rowCount: number;
	    importTime: string;
	    fileName?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.state = source["state"];
	        this.label = source["label"];
	        this.tooltip = source["tooltip"];
	        this.rowCount = source["rowCount"];
	        this.importTime = source["importTime"];
	        this.fileName = source["fileName"];
	    }
	}
	export class AlignResult {
	    failed: boolean;
	    title: string;
	    message: string;
	    log: string;
	    aligned: string[];
	    failures: string[];
	    imports: Record<string, ImportState>;
	
	    static createFrom(source: any = {}) {
	        return new AlignResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.failed = source["failed"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.log = source["log"];
	        this.aligned = source["aligned"];
	        this.failures = source["failures"];
	        this.imports = this.convertValues(source["imports"], ImportState, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BackupResult {
	    title: string;
	    message: string;
	    failed: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new BackupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.message = source["message"];
	        this.failed = source["failed"];
	        this.path = source["path"];
	    }
	}
	export class CleanupResult {
	    failed: boolean;
	    title: string;
	    message: string;
	    log: string;
	    deleted: string[];
	
	    static createFrom(source: any = {}) {
	        return new CleanupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.failed = source["failed"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.log = source["log"];
	        this.deleted = source["deleted"];
	    }
	}
	export class ClearResult {
	    log: string;
	    title: string;
	    message: string;
	    cleared: string[];
	    failed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClearResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.log = source["log"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.cleared = source["cleared"];
	        this.failed = source["failed"];
	    }
	}
	export class ConsolidateResult {
	    log: string;
	    title: string;
	    message: string;
	    failed: boolean;
	    columns: string[];
	    rows: string[][];
	    statuses: string[];
	    conflicts: string[][];
	    notice: string;
	    rowConflicts: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConsolidateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.log = source["log"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.failed = source["failed"];
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.statuses = source["statuses"];
	        this.conflicts = source["conflicts"];
	        this.notice = source["notice"];
	        this.rowConflicts = source["rowConflicts"];
	    }
	}
	export class DiagnosticsResult {
	    failed: boolean;
	    title: string;
	    message: string;
	    log: string;
	    path: string;
	    entries: string[];
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.failed = source["failed"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.log = source["log"];
	        this.path = source["path"];
	        this.entries = source["entries"];
	    }
	}
	export class ExportResult {
	    log: string;
	    title: string;
	    message: string;
	    failed: boolean;
	    path: string;
	    sharedPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.log = source["log"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.failed = source["failed"];
	        this.path = source["path"];
	        this.sharedPath = source["sharedPath"];
	    }
	}
	export class ExportTarget {
	    defaultName: string;
	    target: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultName = source["defaultName"];
	        this.target = source["target"];
	    }
	}
	export class ImportResult {
	    source: string;
	    fileName: string;
	    rowCount: number;
	    state: ImportState;
	    log: string;
	    title: string;
	    message: string;
	    failed: boolean;
	    publishedVersion: string;
	    droppedExpired: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.fileName = source["fileName"];
	        this.rowCount = source["rowCount"];
	        this.state = this.convertValues(source["state"], ImportState);
	        this.log = source["log"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.failed = source["failed"];
	        this.publishedVersion = source["publishedVersion"];
	        this.droppedExpired = source["droppedExpired"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LastConsolidation {
	    at: string;
	    by: string;
	    versions: Record<string, string>;
	    result: string;
	    counts: share.Counts;
	
	    static createFrom(source: any = {}) {
	        return new LastConsolidation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	        this.by = source["by"];
	        this.versions = source["versions"];
	        this.result = source["result"];
	        this.counts = this.convertValues(source["counts"], share.Counts);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionInfo {
	    name: string;
	    role: string;
	    roleLabel: string;
	    username: string;
	    machine: string;
	    at: string;
	    singleUser: boolean;
	    shareReady: boolean;
	    shareDetail: string;
	    actions: string[];
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.role = source["role"];
	        this.roleLabel = source["roleLabel"];
	        this.username = source["username"];
	        this.machine = source["machine"];
	        this.at = source["at"];
	        this.singleUser = source["singleUser"];
	        this.shareReady = source["shareReady"];
	        this.shareDetail = source["shareDetail"];
	        this.actions = source["actions"];
	    }
	}
	export class LoginResult {
	    failed: boolean;
	    title: string;
	    message: string;
	    log: string;
	    session?: SessionInfo;
	
	    static createFrom(source: any = {}) {
	        return new LoginResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.failed = source["failed"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.log = source["log"];
	        this.session = this.convertValues(source["session"], SessionInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LoginRole {
	    key: string;
	    label: string;
	    hint: string;
	    needsPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LoginRole(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.hint = source["hint"];
	        this.needsPassword = source["needsPassword"];
	    }
	}
	export class NotificationPreview {
	    source: string;
	    to: string[];
	    subject: string;
	    body: string;
	    transport: string;
	    mode: string;
	    ready: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new NotificationPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.to = source["to"];
	        this.subject = source["subject"];
	        this.body = source["body"];
	        this.transport = source["transport"];
	        this.mode = source["mode"];
	        this.ready = source["ready"];
	        this.reason = source["reason"];
	    }
	}
	export class NotificationResult {
	    failed: boolean;
	    title: string;
	    message: string;
	    to?: string[];
	    mode?: string;
	    log: string;
	
	    static createFrom(source: any = {}) {
	        return new NotificationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.failed = source["failed"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.to = source["to"];
	        this.mode = source["mode"];
	        this.log = source["log"];
	    }
	}
	export class ReviewResult {
	    log: string;
	    title: string;
	    message: string;
	    failed: boolean;
	    columns: string[];
	    rows: string[][];
	    total: number;
	    page: number;
	    pageSize: number;
	    pageCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ReviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.log = source["log"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.failed = source["failed"];
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.pageCount = source["pageCount"];
	    }
	}
	export class RunningInfo {
	    owner: string;
	    role: string;
	    since: string;
	
	    static createFrom(source: any = {}) {
	        return new RunningInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.owner = source["owner"];
	        this.role = source["role"];
	        this.since = source["since"];
	    }
	}
	
	export class SettingsChange {
	    path: string[];
	    action?: string;
	    index?: number;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.action = source["action"];
	        this.index = source["index"];
	        this.value = source["value"];
	    }
	}
	export class SettingsNode {
	    key: string;
	    kind: string;
	    value?: string;
	    children?: SettingsNode[];
	    path?: string[];
	    comment?: string;
	    control?: string;
	    options?: string[];
	    label?: string;
	    note?: string;
	    editable?: boolean;
	    reason?: string;
	    listEdit?: boolean;
	    flow?: boolean;
	    alias?: string;
	    style?: string;
	    min?: number;
	    max?: number;
	
	    static createFrom(source: any = {}) {
	        return new SettingsNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.kind = source["kind"];
	        this.value = source["value"];
	        this.children = this.convertValues(source["children"], SettingsNode);
	        this.path = source["path"];
	        this.comment = source["comment"];
	        this.control = source["control"];
	        this.options = source["options"];
	        this.label = source["label"];
	        this.note = source["note"];
	        this.editable = source["editable"];
	        this.reason = source["reason"];
	        this.listEdit = source["listEdit"];
	        this.flow = source["flow"];
	        this.alias = source["alias"];
	        this.style = source["style"];
	        this.min = source["min"];
	        this.max = source["max"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SettingsSaveResult {
	    title: string;
	    message: string;
	    failed: boolean;
	    backup: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.message = source["message"];
	        this.failed = source["failed"];
	        this.backup = source["backup"];
	    }
	}
	export class SettingsTab {
	    key: string;
	    title: string;
	    note: string;
	    path: string;
	    tree: SettingsNode[];
	    raw: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.title = source["title"];
	        this.note = source["note"];
	        this.path = source["path"];
	        this.tree = this.convertValues(source["tree"], SettingsNode);
	        this.raw = source["raw"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SourceStatus {
	    source: string;
	    label: string;
	    canView: boolean;
	    localVersion: string;
	    remoteVersion: string;
	    remoteRows: number;
	    publishedAt: string;
	    publishedBy: string;
	    aligned: boolean;
	    pending: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.label = source["label"];
	        this.canView = source["canView"];
	        this.localVersion = source["localVersion"];
	        this.remoteVersion = source["remoteVersion"];
	        this.remoteRows = source["remoteRows"];
	        this.publishedAt = source["publishedAt"];
	        this.publishedBy = source["publishedBy"];
	        this.aligned = source["aligned"];
	        this.pending = source["pending"];
	    }
	}
	export class ShareOverview {
	    status: share.Status;
	    sources: SourceStatus[];
	    lastConsolidation?: LastConsolidation;
	    autoAlign: boolean;
	    pollSeconds: number;
	    enabled: boolean;
	    running?: RunningInfo;
	
	    static createFrom(source: any = {}) {
	        return new ShareOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = this.convertValues(source["status"], share.Status);
	        this.sources = this.convertValues(source["sources"], SourceStatus);
	        this.lastConsolidation = this.convertValues(source["lastConsolidation"], LastConsolidation);
	        this.autoAlign = source["autoAlign"];
	        this.pollSeconds = source["pollSeconds"];
	        this.enabled = source["enabled"];
	        this.running = this.convertValues(source["running"], RunningInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class State {
	    version: string;
	    operationLog: string;
	    imports: Record<string, ImportState>;
	    busy: boolean;
	    session?: SessionInfo;
	    roles: LoginRole[];
	    username: string;
	    singleUser: boolean;
	    share: share.Status;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.operationLog = source["operationLog"];
	        this.imports = this.convertValues(source["imports"], ImportState, true);
	        this.busy = source["busy"];
	        this.session = this.convertValues(source["session"], SessionInfo);
	        this.roles = this.convertValues(source["roles"], LoginRole);
	        this.username = source["username"];
	        this.singleUser = source["singleUser"];
	        this.share = this.convertValues(source["share"], share.Status);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace share {
	
	export class Counts {
	    ready: number;
	    missing: number;
	    duplicate: number;
	    conflict: number;
	
	    static createFrom(source: any = {}) {
	        return new Counts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.missing = source["missing"];
	        this.duplicate = source["duplicate"];
	        this.conflict = source["conflict"];
	    }
	}
	export class Status {
	    mode: string;
	    healthy: boolean;
	    detail: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.healthy = source["healthy"];
	        this.detail = source["detail"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

