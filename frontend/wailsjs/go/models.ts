export namespace main {
	
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
	    }
	}
	export class ExportResult {
	    log: string;
	    title: string;
	    message: string;
	    failed: boolean;
	    path: string;
	
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
	export class ImportState {
	    source: string;
	    state: string;
	    label: string;
	    tooltip: string;
	    rowCount: number;
	    importTime: string;
	
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
	    html: string;
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
	        this.html = source["html"];
	        this.raw = source["raw"];
	    }
	}
	export class State {
	    version: string;
	    operationLog: string;
	    imports: Record<string, ImportState>;
	    busy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.operationLog = source["operationLog"];
	        this.imports = this.convertValues(source["imports"], ImportState, true);
	        this.busy = source["busy"];
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

