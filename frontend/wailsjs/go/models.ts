export namespace wails {
	
	export class Emitter {
	
	
	    static createFrom(source: any = {}) {
	        return new Emitter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

