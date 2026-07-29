export namespace main {
	
	export class BuildConfig {
	    gameSlug: string;
	    customExe: string;
	    cuePath: string;
	    sourceDir: string;
	    outputPath: string;
	    winePath: string;
	    otvdmPath: string;
	    mcicdaPath: string;
	    win16: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BuildConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameSlug = source["gameSlug"];
	        this.customExe = source["customExe"];
	        this.cuePath = source["cuePath"];
	        this.sourceDir = source["sourceDir"];
	        this.outputPath = source["outputPath"];
	        this.winePath = source["winePath"];
	        this.otvdmPath = source["otvdmPath"];
	        this.mcicdaPath = source["mcicdaPath"];
	        this.win16 = source["win16"];
	    }
	}
	export class ProfileInfo {
	    slug: string;
	    name: string;
	    exe: string;
	    win16: boolean;
	    folderSource: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.slug = source["slug"];
	        this.name = source["name"];
	        this.exe = source["exe"];
	        this.win16 = source["win16"];
	        this.folderSource = source["folderSource"];
	    }
	}

}

