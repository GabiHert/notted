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
	export class DeletePlanDTO {
	    audioFiles: number;
	    archiveFolder: string;
	
	    static createFrom(source: any = {}) {
	        return new DeletePlanDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audioFiles = source["audioFiles"];
	        this.archiveFolder = source["archiveFolder"];
	    }
	}
	export class DeleteResultDTO {
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.warning = source["warning"];
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
	export class FormOptionDTO {
	    label: string;
	    description: string;
	    recommended: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FormOptionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.description = source["description"];
	        this.recommended = source["recommended"];
	    }
	}
	export class FormPieceDTO {
	    kind: string;
	    id: string;
	    markdown: string;
	    html: string;
	    question: string;
	    options: FormOptionDTO[];
	    placeholder: string;
	    multiline: boolean;
	    required: boolean;
	    yesLabel: string;
	    noLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new FormPieceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.markdown = source["markdown"];
	        this.html = source["html"];
	        this.question = source["question"];
	        this.options = this.convertValues(source["options"], FormOptionDTO);
	        this.placeholder = source["placeholder"];
	        this.multiline = source["multiline"];
	        this.required = source["required"];
	        this.yesLabel = source["yesLabel"];
	        this.noLabel = source["noLabel"];
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
	export class OpenFormDTO {
	    id: string;
	    meetingId: string;
	    kind: string;
	    title: string;
	    pieces: FormPieceDTO[];
	    queued: number;
	
	    static createFrom(source: any = {}) {
	        return new OpenFormDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.meetingId = source["meetingId"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.pieces = this.convertValues(source["pieces"], FormPieceDTO);
	        this.queued = source["queued"];
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
	export class PermissionsDTO {
	    microphone: string;
	    screen: string;
	
	    static createFrom(source: any = {}) {
	        return new PermissionsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.microphone = source["microphone"];
	        this.screen = source["screen"];
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

