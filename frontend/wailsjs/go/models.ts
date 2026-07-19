export namespace main {
	
	export class VariantStatus {
	    providerId: string;
	    variantIndex: number;
	    label: string;
	    latestVersion: string;
	    foundVersion: string;
	    status: string;
	    error?: string;
	    progress: number;
	
	    static createFrom(source: any = {}) {
	        return new VariantStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.variantIndex = source["variantIndex"];
	        this.label = source["label"];
	        this.latestVersion = source["latestVersion"];
	        this.foundVersion = source["foundVersion"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.progress = source["progress"];
	    }
	}
	export class ProviderInfo {
	    id: string;
	    name: string;
	    category: string;
	    variants: VariantStatus[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.variants = this.convertValues(source["variants"], VariantStatus);
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
	export class VariantSetting {
	    providerId: string;
	    providerName: string;
	    category: string;
	    variantIndex: number;
	    label: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VariantSetting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.providerName = source["providerName"];
	        this.category = source["category"];
	        this.variantIndex = source["variantIndex"];
	        this.label = source["label"];
	        this.enabled = source["enabled"];
	    }
	}

}

