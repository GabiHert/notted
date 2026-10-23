export namespace wails {
	
	export class MeetingDTO {
	    id: string;
	    title: string;
	    startedAt: string;
	    endedAt: string;
	    language: string;
	    clientKey: string;
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
	        this.clientKey = source["clientKey"];
	        this.state = source["state"];
	        this.hasSession = source["hasSession"];
	        this.folder = source["folder"];
	        this.summaryMd = source["summaryMd"];
	        this.suggestionsMd = source["suggestionsMd"];
	    }
	}

}

