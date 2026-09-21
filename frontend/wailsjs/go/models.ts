export namespace main {
	
	export class CandidateInfo {
	    key: string;
	    path: string;
	    name: string;
	    size: number;
	    size_human: string;
	    category: string;
	    confidence: number;
	    reason: string;
	    label: string;
	    modified: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new CandidateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.size_human = source["size_human"];
	        this.category = source["category"];
	        this.confidence = source["confidence"];
	        this.reason = source["reason"];
	        this.label = source["label"];
	        this.modified = source["modified"];
	        this.type = source["type"];
	    }
	}
	export class CategoryInfo {
	    category: string;
	    count: number;
	    total_bytes: number;
	    total_human: string;
	    confidence: number;
	    files: CandidateInfo[];
	
	    static createFrom(source: any = {}) {
	        return new CategoryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.count = source["count"];
	        this.total_bytes = source["total_bytes"];
	        this.total_human = source["total_human"];
	        this.confidence = source["confidence"];
	        this.files = this.convertValues(source["files"], CandidateInfo);
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
	export class DriveInfo {
	    path: string;
	    type: string;
	    removable: boolean;
	    free_bytes: number;
	    total_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DriveInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.type = source["type"];
	        this.removable = source["removable"];
	        this.free_bytes = source["free_bytes"];
	        this.total_bytes = source["total_bytes"];
	    }
	}
	export class MoveOutcome {
	    success: boolean;
	    message: string;
	    succeeded: number;
	    failed: number;
	    freed: number;
	    freed_human: string;
	    details: string[];
	    moved_keys: string[];
	
	    static createFrom(source: any = {}) {
	        return new MoveOutcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.succeeded = source["succeeded"];
	        this.failed = source["failed"];
	        this.freed = source["freed"];
	        this.freed_human = source["freed_human"];
	        this.details = source["details"];
	        this.moved_keys = source["moved_keys"];
	    }
	}
	export class QuarantineItem {
	    id: string;
	    original_path: string;
	    quarantine_path: string;
	    size: number;
	    size_human: string;
	    category: string;
	    label: string;
	    moved_at: string;
	    retention_until: string;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new QuarantineItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.original_path = source["original_path"];
	        this.quarantine_path = source["quarantine_path"];
	        this.size = source["size"];
	        this.size_human = source["size_human"];
	        this.category = source["category"];
	        this.label = source["label"];
	        this.moved_at = source["moved_at"];
	        this.retention_until = source["retention_until"];
	        this.expired = source["expired"];
	    }
	}
	export class ScanReportDTO {
	    generated_at: string;
	    duration_ms: number;
	    roots: string[];
	    files_scanned: number;
	    dirs_scanned: number;
	    total_bytes: number;
	    total_human: string;
	    candidates: CategoryInfo[];
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScanReportDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = source["generated_at"];
	        this.duration_ms = source["duration_ms"];
	        this.roots = source["roots"];
	        this.files_scanned = source["files_scanned"];
	        this.dirs_scanned = source["dirs_scanned"];
	        this.total_bytes = source["total_bytes"];
	        this.total_human = source["total_human"];
	        this.candidates = this.convertValues(source["candidates"], CategoryInfo);
	        this.errors = source["errors"];
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
	export class ScanResult {
	    success: boolean;
	    message: string;
	    report: ScanReportDTO;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.report = this.convertValues(source["report"], ScanReportDTO);
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

