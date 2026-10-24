export namespace wails {
	
	export class ArchiveCheckDTO {
	    path: string;
	    missing: string[];
	    inRepo: boolean;
	    nested: boolean;
	    repoRoot: string;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveCheckDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.missing = source["missing"];
	        this.inRepo = source["inRepo"];
	        this.nested = source["nested"];
	        this.repoRoot = source["repoRoot"];
	        this.ready = source["ready"];
	    }
	}
	export class CommitDTO {
	    sha: string;
	    pushed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CommitDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.pushed = source["pushed"];
	    }
	}
	export class ExportDTO {
	    folder: string;
	    files: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExportDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.files = source["files"];
	    }
	}
	export class InstructionFileDTO {
	    path: string;
	    kind: string;
	    readOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InstructionFileDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class MeetingDTO {
	    id: string;
	    title: string;
	    startedAt: string;
	    endedAt: string;
	    language: string;
	    projectKey: string;
	    state: string;
	    hasSession: boolean;
	    folder: string;
	    summaryMd: string;
	    suggestionsMd: string;
	
	    static createFrom(source: any = {}) {
	        return new MeetingDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.language = source["language"];
	        this.projectKey = source["projectKey"];
	        this.state = source["state"];
	        this.hasSession = source["hasSession"];
	        this.folder = source["folder"];
	        this.summaryMd = source["summaryMd"];
	        this.suggestionsMd = source["suggestionsMd"];
	    }
	}
	export class ModelStatusDTO {
	    language: string;
	    ready: boolean;
	    missing: string;
	    downloadBytes: number;
	    downloading: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.ready = source["ready"];
	        this.missing = source["missing"];
	        this.downloadBytes = source["downloadBytes"];
	        this.downloading = source["downloading"];
	    }
	}
	export class SegmentDTO {
	    id: number;
	    track: string;
	    startMs: number;
	    endMs: number;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new SegmentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.track = source["track"];
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
	        this.text = source["text"];
	    }
	}
	export class SettingsDTO {
	    profile: string;
	    checkoutPath: string;
	    projectRoots: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.checkoutPath = source["checkoutPath"];
	        this.projectRoots = source["projectRoots"];
	    }
	}
	export class UpdateStatusDTO {
	    sourcePath: string;
	    installedApp: string;
	    signing: boolean;
	    state: string;
	    message: string;
	    lines: string[];
	    seq: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.installedApp = source["installedApp"];
	        this.signing = source["signing"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.lines = source["lines"];
	        this.seq = source["seq"];
	    }
	}

}

