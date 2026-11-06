export namespace agents {

	export class Workspace {
	    id: string;
	    name: string;
	    description: string;
	    scope: string;
	    configPath: string;
	    detected: boolean;
	    installed: boolean;
	    canInstall: boolean;
	    backupPath?: string;
	    commandPreview: string[];
	    manual: string;
	    notes?: string[];

	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.scope = source["scope"];
	        this.configPath = source["configPath"];
	        this.detected = source["detected"];
	        this.installed = source["installed"];
	        this.canInstall = source["canInstall"];
	        this.backupPath = source["backupPath"];
	        this.commandPreview = source["commandPreview"];
	        this.manual = source["manual"];
	        this.notes = source["notes"];
	    }
	}

}

export namespace desktop {

	export class LoadingState {
	    active: boolean;
	    operation: string;
	    title: string;
	    detail: string;
	    percent?: number;
	    totalTests?: number;
	    providerTests?: number;
	    current?: number;
	    total?: number;
	    providerName?: string;
	    latestTest?: string;
	    startedAt?: string;

	    static createFrom(source: any = {}) {
	        return new LoadingState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.operation = source["operation"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	        this.percent = source["percent"];
	        this.totalTests = source["totalTests"];
	        this.providerTests = source["providerTests"];
	        this.current = source["current"];
	        this.total = source["total"];
	        this.providerName = source["providerName"];
	        this.latestTest = source["latestTest"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class ProjectEntry {
	    root: string;
	    name: string;
	    active: boolean;
	    configured: boolean;
	    lastOpened: string;
	    providerCount: number;
	    testCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.name = source["name"];
	        this.active = source["active"];
	        this.configured = source["configured"];
	        this.lastOpened = source["lastOpened"];
	        this.providerCount = source["providerCount"];
	        this.testCount = source["testCount"];
	    }
	}
	export class SetupRequest {
	    projectRoot: string;
	    force: boolean;
	    autoDiscover: boolean;
	    widgetMode: string;
	    alwaysOnTop: boolean;
	    topmostMode: string;
	
	    static createFrom(source: any = {}) {
	        return new SetupRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectRoot = source["projectRoot"];
	        this.force = source["force"];
	        this.autoDiscover = source["autoDiscover"];
	        this.widgetMode = source["widgetMode"];
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.topmostMode = source["topmostMode"];
	    }
	}
	export class SetupResponse {
	    config: domain.ProjectConfig;
	    snapshot: domain.Snapshot;
	    discoverWarning?: string;
	
	    static createFrom(source: any = {}) {
	        return new SetupResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], domain.ProjectConfig);
	        this.snapshot = this.convertValues(source["snapshot"], domain.Snapshot);
	        this.discoverWarning = source["discoverWarning"];
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
	export class StartupState {
	    configured: boolean;
	    projectRoot: string;
	    projectName: string;
	    suggestedProjectRoot: string;
	    preferencePath: string;
	    configPath: string;
	    providers: domain.Provider[];
	    projects: ProjectEntry[];
	    ui: domain.UIConfig;
	    setupError?: string;
	    detectedPreview: domain.ProjectConfig;
	
	    static createFrom(source: any = {}) {
	        return new StartupState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.projectRoot = source["projectRoot"];
	        this.projectName = source["projectName"];
	        this.suggestedProjectRoot = source["suggestedProjectRoot"];
	        this.preferencePath = source["preferencePath"];
	        this.configPath = source["configPath"];
	        this.providers = this.convertValues(source["providers"], domain.Provider);
	        this.projects = this.convertValues(source["projects"], ProjectEntry);
	        this.ui = this.convertValues(source["ui"], domain.UIConfig);
	        this.setupError = source["setupError"];
	        this.detectedPreview = this.convertValues(source["detectedPreview"], domain.ProjectConfig);
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
	export class WorkspaceResponse {
	    startup: StartupState;
	    snapshot: domain.Snapshot;

	    static createFrom(source: any = {}) {
	        return new WorkspaceResponse(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startup = this.convertValues(source["startup"], StartupState);
	        this.snapshot = this.convertValues(source["snapshot"], domain.Snapshot);
	    }

		convertValues(a: any, classs: any): any {
		    if (!a) return a;
		    if (a.slice && a.map) return (a as any[]).map(elem => this.convertValues(elem, classs));
		    if ("object" === typeof a) return new classs(a);
		    return a;
		}
	}

}

export namespace domain {
	
	export class CommandSpec {
	    exe: string;
	    args?: string[];
	    env?: Record<string, string>;
	    workingDir?: string;
	    timeoutSeconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new CommandSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exe = source["exe"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.workingDir = source["workingDir"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	    }
	}
	export class DiscoverSpec {
	    command: CommandSpec;
	    parser: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscoverSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = this.convertValues(source["command"], CommandSpec);
	        this.parser = source["parser"];
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
	export class Filter {
	    providerIds?: string[];
	    providerKinds?: string[];
	    layers?: string[];
	    modules?: string[];
	    statuses?: string[];
	    tags?: string[];
	    query?: string;
	    limit?: number;
	    offset?: number;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerIds = source["providerIds"];
	        this.providerKinds = source["providerKinds"];
	        this.layers = source["layers"];
	        this.modules = source["modules"];
	        this.statuses = source["statuses"];
	        this.tags = source["tags"];
	        this.query = source["query"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class ResultSpec {
	    format: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ResultSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.path = source["path"];
	    }
	}
	export class RunSpec {
	    all: CommandSpec;
	    test?: CommandSpec;
	
	    static createFrom(source: any = {}) {
	        return new RunSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.all = this.convertValues(source["all"], CommandSpec);
	        this.test = this.convertValues(source["test"], CommandSpec);
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
	export class Provider {
	    id: string;
	    name: string;
	    kind: string;
	    layer?: string;
	    root: string;
	    enabled: boolean;
	    tags?: string[];
	    discover: DiscoverSpec;
	    run: RunSpec;
	    results?: ResultSpec;
	
	    static createFrom(source: any = {}) {
	        return new Provider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.layer = source["layer"];
	        this.root = source["root"];
	        this.enabled = source["enabled"];
	        this.tags = source["tags"];
	        this.discover = this.convertValues(source["discover"], DiscoverSpec);
	        this.run = this.convertValues(source["run"], RunSpec);
	        this.results = this.convertValues(source["results"], ResultSpec);
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
	export class UIConfig {
	    theme: string;
	    widgetMode: string;
	    alwaysOnTop: boolean;
	    topmostMode?: string;
	    startMinimized: boolean;
	    compactWidth: number;
	    compactHeight: number;
	    compactExpandedHeight: number;
	    compactX?: number;
	    compactY?: number;
	    compactPositionSet?: boolean;
	    managerWidth: number;
	    managerHeight: number;
	
	    static createFrom(source: any = {}) {
	        return new UIConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.widgetMode = source["widgetMode"];
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.topmostMode = source["topmostMode"];
	        this.startMinimized = source["startMinimized"];
	        this.compactWidth = source["compactWidth"];
	        this.compactHeight = source["compactHeight"];
	        this.compactExpandedHeight = source["compactExpandedHeight"];
	        this.compactX = source["compactX"];
	        this.compactY = source["compactY"];
	        this.compactPositionSet = source["compactPositionSet"];
	        this.managerWidth = source["managerWidth"];
	        this.managerHeight = source["managerHeight"];
	    }
	}
	export class ProjectConfig {
	    version: number;
	    name: string;
	    workspace: string;
	    ui: UIConfig;
	    providers: Provider[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.name = source["name"];
	        this.workspace = source["workspace"];
	        this.ui = this.convertValues(source["ui"], UIConfig);
	        this.providers = this.convertValues(source["providers"], Provider);
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
	
	
	export class RunItem {
	    id: string;
	    runId: string;
	    providerId: string;
	    providerName: string;
	    providerKind: string;
	    testId?: string;
	    testName: string;
	    commandPreview: string;
	    status: string;
	    // Go type: time
	    startedAt?: any;
	    // Go type: time
	    finishedAt?: any;
	    durationMillis?: number;
	    estimatedMillis?: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.runId = source["runId"];
	        this.providerId = source["providerId"];
	        this.providerName = source["providerName"];
	        this.providerKind = source["providerKind"];
	        this.testId = source["testId"];
	        this.testName = source["testName"];
	        this.commandPreview = source["commandPreview"];
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.durationMillis = source["durationMillis"];
	        this.estimatedMillis = source["estimatedMillis"];
	        this.error = source["error"];
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
	export class Run {
	    id: string;
	    status: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    startedAt?: any;
	    // Go type: time
	    finishedAt?: any;
	    items: RunItem[];
	    currentIndex: number;
	    passed: number;
	    failed: number;
	    skipped: number;
	    cancelled: number;
	    total: number;
	    logPath?: string;
	    diagnosticsPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.items = this.convertValues(source["items"], RunItem);
	        this.currentIndex = source["currentIndex"];
	        this.passed = source["passed"];
	        this.failed = source["failed"];
	        this.skipped = source["skipped"];
	        this.cancelled = source["cancelled"];
	        this.total = source["total"];
	        this.logPath = source["logPath"];
	        this.diagnosticsPath = source["diagnosticsPath"];
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
	
	
	export class TestCase {
	    id: string;
	    providerId: string;
	    providerName: string;
	    providerKind: string;
	    name: string;
	    fullName: string;
	    selector: string;
	    jestProject?: string;
	    vitestProject?: string;
	    file?: string;
	    line?: number;
	    layer?: string;
	    module?: string;
	    tags?: string[];
	    lastStatus: string;
	    lastDurationMillis?: number;
	    lastRunId?: string;
	    lastFailure?: string;
	    // Go type: time
	    discoveredAt: any;
	
	    static createFrom(source: any = {}) {
	        return new TestCase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.providerId = source["providerId"];
	        this.providerName = source["providerName"];
	        this.providerKind = source["providerKind"];
	        this.name = source["name"];
	        this.fullName = source["fullName"];
	        this.selector = source["selector"];
	        this.jestProject = source["jestProject"];
	        this.vitestProject = source["vitestProject"];
	        this.file = source["file"];
	        this.line = source["line"];
	        this.layer = source["layer"];
	        this.module = source["module"];
	        this.tags = source["tags"];
	        this.lastStatus = source["lastStatus"];
	        this.lastDurationMillis = source["lastDurationMillis"];
	        this.lastRunId = source["lastRunId"];
	        this.lastFailure = source["lastFailure"];
	        this.discoveredAt = this.convertValues(source["discoveredAt"], null);
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
	export class Snapshot {
	    projectName: string;
	    providerCount: number;
	    testCount: number;
	    queuedCount: number;
	    paused: boolean;
	    running?: Run;
	    recentRuns: Run[];
	    tests?: TestCase[];
	    activeProviderIds: string[];
	    // Go type: time
	    generatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectName = source["projectName"];
	        this.providerCount = source["providerCount"];
	        this.testCount = source["testCount"];
	        this.queuedCount = source["queuedCount"];
	        this.paused = source["paused"];
	        this.running = this.convertValues(source["running"], Run);
	        this.recentRuns = this.convertValues(source["recentRuns"], Run);
	        this.tests = this.convertValues(source["tests"], TestCase);
	        this.activeProviderIds = source["activeProviderIds"];
	        this.generatedAt = this.convertValues(source["generatedAt"], null);
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

export namespace manager {
	
	export class Diagnostics {
	    run?: domain.Run;
	    log: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Diagnostics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], domain.Run);
	        this.log = source["log"];
	        this.path = source["path"];
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
	export class RunRequest {
	    testIds?: string[];
	    providerIds?: string[];
	    filter?: domain.Filter;
	
	    static createFrom(source: any = {}) {
	        return new RunRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.testIds = source["testIds"];
	        this.providerIds = source["providerIds"];
	        this.filter = this.convertValues(source["filter"], domain.Filter);
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
