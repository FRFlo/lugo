const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const vscode = require("vscode");
const { LanguageClient, ErrorAction, CloseAction, State } = require("vscode-languageclient/node");
const { PostHog } = require("posthog-node");
const { discoverResources, parseManifest, renderResource } = require("./fivem_resources");
const { createTableHoverProvider } = require("./hover_table");
const { registerMcp } = require("./mcp");

const LUA_DOCUMENT_SELECTOR = [
	{ scheme: "file", language: "lua" },
	{ scheme: "untitled", language: "lua" },
];

const DEFAULT_LIMIT = 50;
const STORAGE_KEY = "lugo.telemetry.buffer.v1";
const REDACTED = "[REDACTED]";
const SECRET_KEY = /token|secret|password|authorization|cookie|apikey|api[_-]?key|source|contents?|text|args?|arguments?|path|uri|file/i;
const PATH_VALUE = /(?:[A-Za-z]:[\\/]|(?:\\\\|\/)|\.\.\\|\/)[^\s]*/;

function redact(value, key = "", depth = 0) {
	if (depth > 6 || value === null || value === undefined) return value;
	if (SECRET_KEY.test(key)) return REDACTED;
	if (typeof value === "string") {
		if (PATH_VALUE.test(value) || value.length > 512) return REDACTED;
		return value;
	}
	if (typeof value === "number") return Number.isFinite(value) ? value : REDACTED;
	if (typeof value === "boolean") return value;
	if (Array.isArray(value)) return value.slice(0, 20).map(item => redact(item, key, depth + 1));
	if (typeof value === "object") {
		const result = {};
		for (const [childKey, childValue] of Object.entries(value).slice(0, 40)) {
			result[childKey] = redact(childValue, childKey, depth + 1);
		}
		return result;
	}
	return REDACTED;
}

// Error messages can contain document contents, paths, or command arguments.
// Telemetry receives only a short, allowlisted error identity.
function errorMetadata(error) {
	const name = typeof error?.name === "string" && /^[A-Za-z][A-Za-z0-9_]{0,63}$/.test(error.name) ? error.name : "Error";
	const code = typeof error?.code === "string" && /^[A-Za-z0-9_-]{1,32}$/.test(error.code) ? error.code : undefined;
	return code ? { error_type: name, error_code: code } : { error_type: name };
}

function elapsedMilliseconds(startedAt) {
	return Math.max(0, Math.min(3_600_000, Date.now() - startedAt));
}

class Telemetry {
	constructor({ posthog, distinctId, storage, enabled = true, limit = DEFAULT_LIMIT, now = () => Date.now(), id = () => crypto.randomUUID() }) {
		this.posthog = posthog;
		this.distinctId = distinctId || "anonymous";
		this.storage = storage;
		this.enabled = enabled;
		this.limit = limit;
		this.now = now;
		this.id = id;
		this.sessionId = id();
		this.traceId = id();
		this.spanId = id();
		this.restartCount = 0;
		this.buffer = this.#load();
	}

	#load() {
		try {
			const saved = this.storage?.get(STORAGE_KEY, []);
			return Array.isArray(saved) ? saved.slice(-this.limit) : [];
		} catch { return []; }
	}

	#persist() {
		try { void this.storage?.update(STORAGE_KEY, this.buffer.slice(-this.limit)); } catch {}
	}

	setEnabled(enabled) {
		this.enabled = enabled !== false;
		if (!this.enabled) {
			this.buffer = [];
			this.#persist();
		}
	}

	record(event, properties = {}) {
		if (!this.enabled) return;
		const payload = {
			event,
			timestamp: new Date(this.now()).toISOString(),
			session_id: this.sessionId,
			trace_id: this.traceId,
			span_id: this.spanId,
			...redact(properties),
		};
		this.buffer.push(payload);
		if (this.buffer.length > this.limit) this.buffer.splice(0, this.buffer.length - this.limit);
		this.#persist();
		try {
			this.posthog?.capture({ distinctId: this.distinctId, event, properties: payload });
		} catch {}
	}

	lspStarted() { this.record("lsp_started", { restart_count: this.restartCount }); }
	lspRestart(reason = "closed") {
		this.restartCount++;
		this.record("lsp_restart", { restart_count: this.restartCount, reason });
	}
	lspError(error, method, count) {
		this.record("lsp_error", { ...errorMetadata(error), method: typeof method === "string" ? method.slice(0, 80) : "unknown", count: Math.max(0, Math.min(100, Number(count) || 0)), restart_count: this.restartCount });
	}
	lspCrash(error) {
		this.record("lsp_crash", { ...errorMetadata(error), restart_count: this.restartCount, recent_events: this.buffer.slice(-5).map(item => item.event) });
	}
	traceContext() { return { traceId: this.traceId, spanId: this.spanId }; }

	async shutdown() {
		try { await this.posthog?.shutdown(); } catch {}
	}
}


class FiveMResourceItem extends vscode.TreeItem {
	constructor(resource) {
		super(resource.name, vscode.TreeItemCollapsibleState.None);
		this.description = `${resource.profile} · ${resource.diagnostics} diagnostics`;
		this.tooltip = renderResource(resource);
		this.resource = resource;
		this.command = { command: "vscode.open", title: "Open manifest", arguments: [vscode.Uri.file(resource.manifestPath)] };
		this.iconPath = new vscode.ThemeIcon(resource.diagnostics ? "warning" : "server-process");
	}
}

class FiveMResourceProvider {
	constructor() {
		this.resources = [];
		this.refreshVersion = 0;
		this.manifestVersions = new Map();
		this.removedManifests = new Set();
		this._onDidChangeTreeData = new vscode.EventEmitter();
		this.onDidChangeTreeData = this._onDidChangeTreeData.event;
	}
	async refresh() {
		const version = ++this.refreshVersion;
		this.manifestVersions.clear();
		this.removedManifests.clear();
		const diagnostics = vscode.languages.getDiagnostics().map(([uri, values]) => ({ path: uri.fsPath, count: values.length }));
		this.resources = [];
		this._onDidChangeTreeData.fire();
		const workspaceFolders = (vscode.workspace.workspaceFolders || []).map(folder => folder.uri.fsPath);
		let resources;
		try {
			resources = await discoverResources(workspaceFolders, diagnostics, resource => {
			if (version !== this.refreshVersion) return;
			this.resources.push(resource);
			this.resources.sort((a, b) => a.name.localeCompare(b.name) || a.manifestPath.localeCompare(b.manifestPath));
			this._onDidChangeTreeData.fire();
			}, () => version !== this.refreshVersion);
		} catch (error) {
			telemetry?.record("resource_discovery_failed", errorMetadata(error));
			throw error;
		}
		if (resources.length === 0 && workspaceFolders.length > 0) {
			telemetry?.record("resource_discovery_skipped", { reason: "no_resources_found", workspace_count: Math.min(workspaceFolders.length, 100) });
		}
		if (version === this.refreshVersion) {
			this.resources = resources;
			this.updateDiagnostics();
		}
		return this.resources;
	}
	async refreshManifest(manifestPath) {
		if (!this.resources.some(resource => resource.manifestPath === manifestPath)) return false;
		const scanVersion = this.refreshVersion;
		const version = (this.manifestVersions.get(manifestPath) || 0) + 1;
		this.manifestVersions.set(manifestPath, version);
		let content;
		try {
			content = await fs.promises.readFile(manifestPath, "utf8");
		} catch {
			return scanVersion !== this.refreshVersion || this.manifestVersions.get(manifestPath) !== version;
		}
		if (scanVersion !== this.refreshVersion || this.manifestVersions.get(manifestPath) !== version) return true;
		const index = this.resources.findIndex(resource => resource.manifestPath === manifestPath);
		if (index < 0) return false;
		this.resources[index] = parseManifest(content, manifestPath);
		this.resources.sort((a, b) => a.name.localeCompare(b.name) || a.manifestPath.localeCompare(b.manifestPath));
		this.updateDiagnostics();
		return true;
	}
	removeManifest(manifestPath) {
		const index = this.resources.findIndex(resource => resource.manifestPath === manifestPath);
		if (index < 0 && !this.removedManifests.has(manifestPath)) return false;
		this.manifestVersions.set(manifestPath, (this.manifestVersions.get(manifestPath) || 0) + 1);
		this.removedManifests.add(manifestPath);
		if (index >= 0) {
			this.resources.splice(index, 1);
			this._onDidChangeTreeData.fire();
		}
		return true;
	}
	async addManifest(manifestPath, knownWorkspaceRoot = false) {
		if (this.resources.some(resource => resource.manifestPath === manifestPath)) return this.refreshManifest(manifestPath);
		const parent = path.dirname(manifestPath);
		if (!knownWorkspaceRoot && !this.removedManifests.has(manifestPath) && !this.resources.some(resource => path.dirname(resource.manifestPath) === parent)) return false;
		const scanVersion = this.refreshVersion;
		const version = (this.manifestVersions.get(manifestPath) || 0) + 1;
		this.manifestVersions.set(manifestPath, version);
		let content;
		try { content = await fs.promises.readFile(manifestPath, "utf8"); } catch {
			return scanVersion !== this.refreshVersion || this.manifestVersions.get(manifestPath) !== version;
		}
		if (scanVersion !== this.refreshVersion || this.manifestVersions.get(manifestPath) !== version) return true;
		this.removedManifests.delete(manifestPath);
		this.resources.push(parseManifest(content, manifestPath));
		this.resources.sort((a, b) => a.name.localeCompare(b.name) || a.manifestPath.localeCompare(b.manifestPath));
		this.updateDiagnostics();
		return true;
	}
	updateDiagnostics() {
		const diagnostics = vscode.languages.getDiagnostics().map(([uri, values]) => ({ path: uri.fsPath, count: values.length }));
		for (const resource of this.resources) {
			const root = path.dirname(resource.manifestPath) + path.sep;
			resource.diagnostics = diagnostics.filter(item => item.path === resource.manifestPath || item.path.startsWith(root)).reduce((total, item) => total + item.count, 0);
		}
		this._onDidChangeTreeData.fire();
	}
	getTreeItem(resource) { return new FiveMResourceItem(resource); }
	getChildren() { return this.resources; }
	dispose() { this.refreshVersion++; this._onDidChangeTreeData.dispose(); }
}

const posthogClient = new PostHog("phc_AtCceYjFoZzdnFgfKNMGArJGbLMyFzzqvjBx7SQCou6k", { host: "https://eu.i.posthog.com" });

let telemetry;
let client, restarting, indexing, debounce;
let updateWorkspaceStatus = () => {};
const COMMAND_CANCELLED = Symbol("command_cancelled");

async function runCommand(command, handler) {
	telemetry?.record("command_invoked", { command });
	try {
		const result = await handler();
		telemetry?.record(result === COMMAND_CANCELLED ? "command_cancelled" : "command_succeeded", { command });
		return result;
	} catch (error) {
		telemetry?.record("command_failed", { command, ...errorMetadata(error) });
		throw error;
	}
}

const debugExportCategories = [
	{
		label: "Tokens",
		category: "tokens",
		description: "All lexer tokens with byte offsets and text",
		picked: true,
	},
	{
		label: "Identifiers",
		category: "identifiers",
		description: "Identifier-only token stream for name diffs",
		picked: true,
	},
	{
		label: "AST nodes",
		category: "ast",
		description: "Flat arena nodes, comments, ranges, and links",
		picked: true,
	},
	{
		label: "Semantic refs",
		category: "semantic",
		description: "Definitions, references, fields, shadows, reassignments",
		picked: true,
	},
	{
		label: "Global index",
		category: "globalIndex",
		description: "Workspace symbols and cross-document index entries",
		picked: true,
	},
];

async function restartClient(context) {
	if (restarting) return false;
	restarting = true;
	const startedAt = Date.now();
	telemetry?.record("lsp_restart_started");
	try {
		if (client) await client.stop();
		await startClient(context);
		telemetry?.record("lsp_restart_succeeded", { duration_ms: elapsedMilliseconds(startedAt) });
		return true;
	} catch (error) {
		telemetry?.record("lsp_restart_failed", { ...errorMetadata(error), duration_ms: elapsedMilliseconds(startedAt) });
		void vscode.window.showErrorMessage("Lugo LSP could not be started. See the Output panel for details.");
		return false;
	} finally {
		restarting = false;
	}
}

function buildInitializationOptions() {
	const filesConfig = vscode.workspace.getConfiguration("files"),
		searchConfig = vscode.workspace.getConfiguration("search"),
		lugoConfig = vscode.workspace.getConfiguration("lugo");

	let ignoreGlobs = [...(lugoConfig.get("workspace.ignoreGlobs") || [])];

	for (const nativeExcludes of [filesConfig.get("exclude") || {}, searchConfig.get("exclude") || {}]) {
		for (const [key, val] of Object.entries(nativeExcludes)) {
			if (val === true) {
				ignoreGlobs.push(key);
			}
		}
	}

	ignoreGlobs = [...new Set(ignoreGlobs)];

	return {
		libraryPaths: configuredLibraryPaths(),
		ignoreGlobs: ignoreGlobs,
		knownGlobals: lugoConfig.get("environment.knownGlobals") || [],
		frameworkAdapters: lugoConfig.get("fivem.frameworkAdapters") || [],
		fiveMServerConfig: lugoConfig.get("fivem.serverConfig") || "",
		sqlAdapters: lugoConfig.get("fivem.sqlAdapters") || [],
		bannedSymbols: lugoConfig.get("diagnostics.bannedSymbols") || {},
		maxFileSizeMB: lugoConfig.get("workspace.maxFileSizeMB") ?? 4,
		telemetryEnabled: lugoConfig.get("telemetry.enabled") !== false,
		...(telemetry ? {
			telemetryTraceId: telemetry.traceContext().traceId,
			telemetrySpanId: telemetry.traceContext().spanId,
		} : {}),

		parserMaxErrors: lugoConfig.get("parser.maxErrors") ?? 50,

		diagUndefinedGlobals: lugoConfig.get("diagnostics.undefinedGlobals") !== false,
		diagImplicitGlobals: lugoConfig.get("diagnostics.implicitGlobals") !== false,
		diagUnusedLocal: lugoConfig.get("diagnostics.unused.local") !== false,
		diagUnusedFunction: lugoConfig.get("diagnostics.unused.function") !== false,
		diagUnusedParameter: lugoConfig.get("diagnostics.unused.parameter") !== false,
		diagUnusedLoopVar: lugoConfig.get("diagnostics.unused.loopVar") !== false,
		diagShadowing: lugoConfig.get("diagnostics.shadowing") !== false,
		diagUnreachableCode: lugoConfig.get("diagnostics.unreachableCode") !== false,
		diagAmbiguousReturns: lugoConfig.get("diagnostics.ambiguousReturns") !== false,
		diagDeprecated: lugoConfig.get("diagnostics.deprecated") !== false,
		diagDuplicateField: lugoConfig.get("diagnostics.duplicateField") !== false,
		diagUnbalancedAssignment: lugoConfig.get("diagnostics.unbalancedAssignment") !== false,
		diagDuplicateLocal: lugoConfig.get("diagnostics.duplicateLocal") !== false,
		diagSelfAssignment: lugoConfig.get("diagnostics.selfAssignment") !== false,
		diagEmptyBlock: lugoConfig.get("diagnostics.emptyBlock") !== false,
		diagFormatString: lugoConfig.get("diagnostics.formatString") !== false,
		diagTypeCheck: lugoConfig.get("diagnostics.typeCheck") === true,
		diagRedundantParameter: lugoConfig.get("diagnostics.redundantParameter") !== false,
		diagRedundantValue: lugoConfig.get("diagnostics.redundantValue") !== false,
		diagRedundantReturn: lugoConfig.get("diagnostics.redundantReturn") !== false,
		diagLoopVarMutation: lugoConfig.get("diagnostics.loopVarMutation") !== false,
		diagIncorrectVararg: lugoConfig.get("diagnostics.incorrectVararg") !== false,
		diagShadowingLoopVar: lugoConfig.get("diagnostics.shadowingLoopVar") !== false,
		diagConstantCondition: lugoConfig.get("diagnostics.constantCondition") !== false,
		diagUnreachableElse: lugoConfig.get("diagnostics.unreachableElse") !== false,
		diagUsedIgnoredVar: lugoConfig.get("diagnostics.usedIgnoredVariable") !== false,

		inlayParamHints: lugoConfig.get("inlayHints.parameterNames") !== false,
		inlaySuppressMatch: lugoConfig.get("inlayHints.suppressWhenArgumentMatchesName") !== false,
		inlayImplicitSelf: lugoConfig.get("inlayHints.implicitSelf") !== false,

		featureDocHighlight: lugoConfig.get("features.documentHighlight") !== false,
		featureHoverEval: lugoConfig.get("features.hoverEvaluation") !== false,
		featureCodeLens: lugoConfig.get("features.codeLens") !== false,
		featureFormatAlerts: lugoConfig.get("features.formatAlerts") !== false,
		featureFormatting: lugoConfig.get("features.formatting") !== false,
		formatOpinionated: lugoConfig.get("features.formatOpinionated") === true,
		suggestFunctionParams: lugoConfig.get("completion.suggestFunctionParams") !== false,

		diagFiveMEventDirection: lugoConfig.get("fivem.diagnostics.eventDirection") !== false,
		diagFiveMEventPayload: lugoConfig.get("fivem.diagnostics.eventPayload") !== false,
		diagFiveMServerConfig: lugoConfig.get("fivem.diagnostics.serverConfig") !== false,
		diagFiveMUnregisteredNetEvent: lugoConfig.get("fivem.diagnostics.unregisteredNetEvent") !== false,
		diagFiveMUnknownEvent: lugoConfig.get("fivem.diagnostics.unknownEvent") !== false,
		diagFiveMUnaccountedFile: lugoConfig.get("fivem.diagnostics.unaccountedFile") !== false,
		diagFiveMUnknownExport: lugoConfig.get("fivem.diagnostics.unknownExport") !== false,
		diagFiveMUnknownResource: lugoConfig.get("fivem.diagnostics.unknownResource") !== false,
		diagFiveMTrustBoundary: lugoConfig.get("fivem.diagnostics.trustBoundary") !== false,
		diagFiveMPerformance: lugoConfig.get("fivem.diagnostics.performance") !== false,
		diagFiveMSQL: lugoConfig.get("fivem.diagnostics.sql") !== false,
	};
}

function scheduleConfigUpdate() {
	clearTimeout(debounce);

	debounce = setTimeout(async () => {
		if (!client?.isRunning()) return;
		try {
			await client.sendNotification("workspace/didChangeConfiguration", {
				settings: buildInitializationOptions(),
			});
			telemetry?.record("configuration_notification_succeeded");
		} catch (error) {
			telemetry?.record("configuration_notification_failed", errorMetadata(error));
			void vscode.window.showWarningMessage("Lugo could not apply the updated configuration.");
		}
	}, 1000);
}

/**
 * Converts a folder URI to a workspace-relative glob pattern.
 * E.g. "C:\project\lib" → "lib/**" when workspace root is "C:\project".
 * Falls back to the absolute path when no workspace folder contains the URI.
 */
function folderUriToWorkspaceGlob(folderUri) {
	const folderPath = folderUri.fsPath,
		workspaceFolder = vscode.workspace.getWorkspaceFolder(folderUri);

	if (workspaceFolder) {
		const relative = path.relative(workspaceFolder.uri.fsPath, folderPath);

		if (relative && !relative.startsWith("..")) {
			return relative.replace(/\\/g, "/") + "/**";
		}
	}

	// Fallback: absolute path with /** suffix
	return folderPath.replace(/\\/g, "/") + "/**";
}

/**
 * Resolves library path globs to absolute paths for the LSP.
 * Workspace-relative globs (e.g. "lib/**") are resolved against each
 * workspace folder root. A trailing /** denotes the directory itself.
 */
function resolveLibraryPathsToAbsolute(globs, workspaceFolders = vscode.workspace.workspaceFolders || []) {
	const resolved = [];

	for (const glob of globs) {
		let pattern = glob.endsWith("/**") ? glob.slice(0, -3) : glob;
		if (path.isAbsolute(glob)) {
			const root = path.parse(glob).root;
			if (pattern === root.slice(0, -1)) pattern = root;
			resolved.push(pattern);
			continue;
		}

		// Workspace-relative glob: resolve against each workspace folder
		for (const folder of workspaceFolders) {
			const absPath = path.join(folder.uri.fsPath, pattern);

			try {
				if (fs.existsSync(absPath) && fs.statSync(absPath).isDirectory()) {
					resolved.push(absPath);
				} else {
					telemetry?.record("library_path_unavailable", { reason: "missing_or_not_directory" });
				}
			} catch (error) {
				telemetry?.record("library_path_unavailable", { reason: "inaccessible", ...errorMetadata(error) });
			}
		}
	}

	return resolved;
}

function configuredLibraryPaths() {
	const folders = vscode.workspace.workspaceFolders || [];
	if (folders.length === 0) {
		return resolveLibraryPathsToAbsolute(vscode.workspace.getConfiguration("lugo").get("workspace.libraryPaths") || [], []);
	}
	const paths = folders.flatMap(folder => resolveLibraryPathsToAbsolute(
		vscode.workspace.getConfiguration("lugo", folder.uri).get("workspace.libraryPaths") || [], [folder]
	));
	return [...new Set(paths)];
}

async function creationMayAffectResources(event) {
	for (const uri of event.files) {
		const name = path.basename(uri.fsPath);
		if (name === "fxmanifest.lua" || name === "__resource.lua") return true;
		try {
			const stat = await vscode.workspace.fs.stat(uri);
			// Unknown types and symlinks are kept conservative: they may be directories.
			if (stat.type !== vscode.FileType.File) return true;
		} catch {
			return true;
		}
	}
	return false;
}

async function deletionMayAffectResources(event, resources, discoveryInProgress, folders) {
	// A partial discovery is not a reliable inventory of resource directories.
	if (discoveryInProgress) return event.files.length > 0;
	if (event.files.some(uri => {
		const deletedPath = path.resolve(uri.fsPath);
		const name = path.basename(deletedPath);
		if (name === "fxmanifest.lua" || name === "__resource.lua") return true;
		return resources.some(resource => {
			const relative = path.relative(deletedPath, resource.manifestPath);
			return relative === "" || (relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative));
		});
	})) return true;
	// A resource can be displayed using a different workspace alias from the
	// path in this event. After deletion the target cannot reliably be resolved,
	// so retain conservative refreshes whenever a workspace root is an alias.
	for (const folder of folders) {
		try {
			if (path.resolve(await fs.promises.realpath(folder.uri.fsPath)) !== path.resolve(folder.uri.fsPath)) return true;
		} catch {
			return true;
		}
	}
	return false;
}

/**
 * Adds a folder to the library paths configuration as a workspace-relative glob.
 */
async function addToLibraryPaths(folderUri) {
	const config = vscode.workspace.getConfiguration("lugo");
	const paths = config.get("workspace.libraryPaths") || [];
	const glob = folderUriToWorkspaceGlob(folderUri);

	if (!paths.includes(glob)) {
		await config.update("workspace.libraryPaths", [...paths, glob], vscode.ConfigurationTarget.Workspace);
		vscode.window.showInformationMessage(`Added "${glob}" to library paths.`);
	} else {
		vscode.window.showInformationMessage(`"${glob}" is already in library paths.`);
	}
}

/**
 * Adds a folder to the ignored globs configuration as a workspace-relative glob.
 */
async function addToIgnoredGlobs(folderUri) {
	const config = vscode.workspace.getConfiguration("lugo");
	const globs = config.get("workspace.ignoreGlobs") || [];
	const glob = folderUriToWorkspaceGlob(folderUri);

	if (!globs.includes(glob)) {
		await config.update("workspace.ignoreGlobs", [...globs, glob], vscode.ConfigurationTarget.Workspace);
		vscode.window.showInformationMessage(`Added "${glob}" to ignored globs.`);
	} else {
		vscode.window.showInformationMessage(`"${glob}" is already in ignored globs.`);
	}
}

async function activate(context) {
	registerMcp(vscode, context);
	context.subscriptions.push(vscode.languages.registerHoverProvider(
		LUA_DOCUMENT_SELECTOR,
		createTableHoverProvider(vscode, () => client),
	));
	const telemetryEnabled = vscode.workspace.getConfiguration("lugo").get("telemetry.enabled") !== false;
	telemetry = new Telemetry({
		posthog: posthogClient,
		distinctId: vscode.env.machineId,
		storage: context.globalState,
		enabled: telemetryEnabled,
	});
	telemetry.record("extension_activated", { os: os.platform(), arch: os.arch() });

	const resourceProvider = new FiveMResourceProvider();
	const resourceView = vscode.window.createTreeView("lugo.fivemResources", { treeDataProvider: resourceProvider, showCollapseAll: false });
	const resourceStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
	const workspaceStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 99);
	updateWorkspaceStatus = progress => {
		if (progress.kind === "end") {
			resourceView.description = undefined;
			workspaceStatus.hide();
			return;
		}
		const message = progress.message || "Preparing workspace...";
		resourceView.description = `Loading · ${message}`;
		workspaceStatus.text = `$(sync~spin) Lugo: ${message}`;
		workspaceStatus.tooltip = "Lugo is indexing the workspace and analyzing diagnostics. Editor features may be delayed.";
		workspaceStatus.show();
	};
	resourceStatus.command = "lugo.fivem.refresh";
	resourceStatus.tooltip = "Refresh FiveM resources";
	let resourceDiscoveries = 0;
	let resourceRefreshDebounce, resourceDiagnosticsDebounce, resourceDisposed = false, resourceRefreshPending = false;
	const refreshResources = async () => {
		const startedAt = Date.now();
		resourceDiscoveries++;
		try {
			const resources = await resourceProvider.refresh();
			if (resourceDisposed) return resources;
			const diagnostics = resources.reduce((total, resource) => total + resource.diagnostics, 0);
			resourceStatus.text = `$(server) FiveM: ${resources.length} resources`;
			resourceStatus.show();
			telemetry?.record("resource_refresh_succeeded", { duration_ms: elapsedMilliseconds(startedAt), resource_count: Math.min(resources.length, 100000), diagnostic_count: Math.min(diagnostics, 1000000) });
			return resources;
		} catch (error) {
			telemetry?.record("resource_refresh_failed", { ...errorMetadata(error), duration_ms: elapsedMilliseconds(startedAt) });
			throw error;
		} finally {
			resourceDiscoveries--;
			if (resourceDiscoveries === 0 && resourceRefreshPending) {
				resourceRefreshPending = false;
				scheduleResourceRefresh();
			}
		}
	};
	const scheduleResourceRefresh = () => {
		if (resourceDisposed) return;
		if (resourceDiscoveries > 0) {
			resourceRefreshPending = true;
			return;
		}
		clearTimeout(resourceRefreshDebounce);
		resourceRefreshDebounce = setTimeout(() => {
			if (resourceDiscoveries > 0) { resourceRefreshPending = true; return; }
			void refreshResources().catch(() => telemetry?.record("resource_refresh_automatic_failed"));
		}, 250);
	};
	const scheduleResourceDiagnostics = () => {
		clearTimeout(resourceDiagnosticsDebounce);
		resourceDiagnosticsDebounce = setTimeout(() => resourceProvider.updateDiagnostics(), 250);
	};
	context.subscriptions.push(resourceProvider, resourceView, resourceStatus, workspaceStatus, { dispose: () => { resourceDisposed = true; updateWorkspaceStatus = () => {}; clearTimeout(resourceRefreshDebounce); clearTimeout(resourceDiagnosticsDebounce); } });
	context.subscriptions.push(vscode.commands.registerCommand("lugo.fivem.refresh", () => runCommand("lugo.fivem.refresh", refreshResources)));
	context.subscriptions.push(vscode.languages.onDidChangeDiagnostics(scheduleResourceDiagnostics));
	let resourceWorkspaceFolders = (vscode.workspace.workspaceFolders || []).map(folder => folder.uri.toString());
	context.subscriptions.push(vscode.workspace.onDidChangeWorkspaceFolders(() => {
		const folders = (vscode.workspace.workspaceFolders || []).map(folder => folder.uri.toString());
		if (folders.length === resourceWorkspaceFolders.length && folders.every((folder, index) => folder === resourceWorkspaceFolders[index])) return;
		resourceWorkspaceFolders = folders;
		scheduleResourceRefresh();
	}));
	const addKnownManifests = async uris => {
		if (resourceDiscoveries > 0 || uris.length === 0 || uris.some(uri => !["fxmanifest.lua", "__resource.lua"].includes(path.basename(uri.fsPath)))) return false;
		const folders = vscode.workspace.workspaceFolders || [];
		const handled = await Promise.all(uris.map(uri => resourceProvider.addManifest(uri.fsPath,
			folders.length === 1 && path.resolve(path.dirname(uri.fsPath)) === path.resolve(folders[0].uri.fsPath))));
		if (resourceDisposed) return true;
		if (!handled.every(Boolean)) return false;
		resourceStatus.text = `$(server) FiveM: ${resourceProvider.resources.length} resources`;
		resourceStatus.show();
		return true;
	};
	context.subscriptions.push(vscode.workspace.onDidCreateFiles(event => {
		void addKnownManifests(event.files).then(async handled => {
			if (!handled && await creationMayAffectResources(event)) scheduleResourceRefresh();
		});
	}));
	const removeKnownManifests = uris => {
		if (resourceDiscoveries > 0 || uris.length === 0 || uris.some(uri => !["fxmanifest.lua", "__resource.lua"].includes(path.basename(uri.fsPath)))) return false;
		if (!uris.every(uri => resourceProvider.resources.some(resource => resource.manifestPath === uri.fsPath) || resourceProvider.removedManifests.has(uri.fsPath))) return false;
		for (const uri of uris) {
			resourceProvider.removeManifest(uri.fsPath);
			// A delayed delete notification may arrive after the file was
			// recreated. Reconcile the inventory if it still exists.
			void fs.promises.stat(uri.fsPath).then(() => scheduleResourceRefresh()).catch(error => {
				if (error.code !== "ENOENT") scheduleResourceRefresh();
			});
		}
		resourceStatus.text = `$(server) FiveM: ${resourceProvider.resources.length} resources`;
		resourceStatus.show();
		return true;
	};
	context.subscriptions.push(vscode.workspace.onDidDeleteFiles(event => {
		if (removeKnownManifests(event.files)) return;
		void deletionMayAffectResources(event, resourceProvider.resources, resourceDiscoveries > 0, vscode.workspace.workspaceFolders || []).then(affects => {
			if (affects || resourceDiscoveries > 0) scheduleResourceRefresh();
		});
	}));
	context.subscriptions.push(vscode.workspace.onDidRenameFiles(event => {
		const removed = { files: event.files.map(file => file.oldUri) };
		void deletionMayAffectResources(removed, resourceProvider.resources, resourceDiscoveries > 0, vscode.workspace.workspaceFolders || []).then(async affects => {
			if (affects || resourceDiscoveries > 0 || await creationMayAffectResources({ files: event.files.map(file => file.newUri) })) scheduleResourceRefresh();
		});
	}));
	const manifestWatcher = vscode.workspace.createFileSystemWatcher("**/{fxmanifest.lua,__resource.lua}");
	context.subscriptions.push(manifestWatcher, manifestWatcher.onDidChange(uri => {
		if (resourceDiscoveries > 0) { scheduleResourceRefresh(); return; }
		void resourceProvider.refreshManifest(uri.fsPath).then(handled => { if (!handled) scheduleResourceRefresh(); }).catch(() => scheduleResourceRefresh());
	}), manifestWatcher.onDidCreate(uri => {
		void addKnownManifests([uri]).then(handled => { if (!handled) scheduleResourceRefresh(); });
	}), manifestWatcher.onDidDelete(uri => {
		if (!removeKnownManifests([uri])) scheduleResourceRefresh();
	}));
	void refreshResources().catch(() => telemetry?.record("resource_refresh_automatic_failed"));

	context.subscriptions.push(
		vscode.workspace.onDidChangeConfiguration(async e => {
			if (e.affectsConfiguration("lugo.telemetry.enabled")) {
				telemetry?.setEnabled(vscode.workspace.getConfiguration("lugo").get("telemetry.enabled") !== false);
			}
			if (e.affectsConfiguration("lugo") || e.affectsConfiguration("files.exclude") || e.affectsConfiguration("search.exclude")) {
				scheduleConfigUpdate();
			}
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.reindex", () => runCommand("lugo.reindex", triggerReindex))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.applySafeFixesWorkspace", () => runCommand("lugo.applySafeFixesWorkspace", () => vscode.commands.executeCommand("lugo.applySafeFixes")))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.applySafeFixesFile", () => runCommand("lugo.applySafeFixesFile", () => {
			const editor = vscode.window.activeTextEditor;
			return editor ? vscode.commands.executeCommand("lugo.applySafeFixes", editor.document.uri.toString()) : COMMAND_CANCELLED;
		}))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.exportDebugData", () => runCommand("lugo.exportDebugData", exportDebugData))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.ignoreDiagnostic", (uriStr, line, rule, isFile) => runCommand("lugo.ignoreDiagnostic", async () => {
			const editor = vscode.window.activeTextEditor;

			if (!editor || editor.document.uri.fsPath !== vscode.Uri.parse(uriStr).fsPath) {
				return COMMAND_CANCELLED;
			}

			let insertLine = line,
				snippetText = "";

			if (isFile) {
				insertLine = 0;
				snippetText = `---@diagnostic disable-file ${rule} - \${1:reason}\n`;
			} else {
				const targetLine = editor.document.lineAt(line),
					indent = targetLine.text.match(/^\s*/)[0];

				insertLine = line;
				snippetText = `${indent}---@diagnostic disable-next-line ${rule} - \${1:reason}\n`;
			}

			await editor.insertSnippet(new vscode.SnippetString(snippetText), new vscode.Position(insertLine, 0));
		}))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.addToLibraryPaths", (clickedFile, selectedFiles) => runCommand("lugo.addToLibraryPaths", async () => {
			// When triggered from context menu, VS Code passes the URI directly.
			// When multiple files are selected, selectedFiles is an array.
			const targets = selectedFiles && selectedFiles.length > 0 ? selectedFiles : [clickedFile];

			for (const target of targets) await addToLibraryPaths(target);
		}))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.addToIgnoredGlobs", (clickedFile, selectedFiles) => runCommand("lugo.addToIgnoredGlobs", async () => {
			const targets = selectedFiles && selectedFiles.length > 0 ? selectedFiles : [clickedFile];

			for (const target of targets) await addToIgnoredGlobs(target);
		}))
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.showReferences", (uriStr, position, locations) => runCommand("lugo.showReferences", () => {
			const uri = vscode.Uri.parse(uriStr),
				pos = new vscode.Position(position.line, position.character);

			const locs = locations.map(
				loc =>
					new vscode.Location(vscode.Uri.parse(loc.uri), new vscode.Range(loc.range.start.line, loc.range.start.character, loc.range.end.line, loc.range.end.character))
			);

			return vscode.commands.executeCommand("editor.action.showReferences", uri, pos, locs);
		}))
	);

	await restartClient(context);
}

async function startClient(context) {
	let initialLibraryPaths;
	let initialSettings;
	let workspaceSync;
	const platform = os.platform(),
		arch = os.arch(),
		ext = platform === "win32" ? ".exe" : "",
		binName = `lugo-${platform}-${arch}${ext}`;

	const serverCommand = path.join(context.extensionPath, "bin", binName);

	if (!fs.existsSync(serverCommand)) {
		telemetry?.record("lsp_binary_missing", { platform, arch });
		void vscode.window.showErrorMessage(`Lugo LSP binary not found for your platform: ${binName}`);
		throw new Error("LspBinaryMissing");
	}

	const serverOptions = {
		run: { command: serverCommand },
		debug: { command: serverCommand },
	};

	let restartCount = 0;

	const clientOptions = {
		middleware: {
			provideHover: () => undefined,
			handleWorkDoneProgress: (token, progress, next) => {
				if (token === "indexing") updateWorkspaceStatus(progress);
				return next();
			},
		},
		documentSelector: LUA_DOCUMENT_SELECTOR,
		synchronize: {
			fileEvents: vscode.workspace.createFileSystemWatcher("**/*.{lua,cfg}"),
		},
		initializationOptions: () => {
			const options = buildInitializationOptions();
			initialLibraryPaths = options.libraryPaths;
			initialSettings = JSON.stringify({ ...options, libraryPaths: undefined });
			workspaceSync = { queue: Promise.resolve(), pendingFolders: 0, libraryPaths: options.libraryPaths, lastConfiguration: JSON.stringify(options) };
			return options;
		},
		errorHandler: {
			error: (error, message, count) => {
				telemetry?.lspError(error, message?.method, count);
				return { action: count <= 3 ? ErrorAction.Continue : ErrorAction.Shutdown };
			},
			closed: () => {
				telemetry?.lspCrash();
				restartCount++;
				if (restartCount <= 5) {
					telemetry?.lspRestart("closed");
					return { action: CloseAction.Restart };
				}
				return { action: CloseAction.DoNotRestart };
			}
		}
	};

	// The language client's initial workspace-folder catch-up bypasses its
	// middleware. Enrich both that event and later events before they are sent.
	class LugoLanguageClient extends LanguageClient {
		sendNotification(type, params) {
			const method = typeof type === "string" ? type : type?.method;
			if (method === "workspace/didChangeWorkspaceFolders") {
				params = { ...params, libraryPaths: configuredLibraryPaths() };
				const state = workspaceSync;
				state.pendingFolders++;
				const sent = state.queue.then(async () => {
					if (state !== workspaceSync) return;
					try {
						await super.sendNotification(type, params);
					} catch (error) {
						if (!this.isRunning() || state !== workspaceSync) throw error;
						await super.sendNotification(type, params);
					}
					if (state === workspaceSync) {
						state.libraryPaths = params.libraryPaths;
						state.lastConfiguration = JSON.stringify({ ...JSON.parse(state.lastConfiguration), libraryPaths: params.libraryPaths });
					}
				}).finally(() => { state.pendingFolders--; });
				state.queue = sent.catch(() => {});
				return sent;
			}
			if (method === "workspace/didChangeConfiguration" && params?.settings) {
				const state = workspaceSync;
				return this.start().then(() => {
					if (state !== workspaceSync) return;
					const sent = state.queue.then(async () => {
						if (state !== workspaceSync) return;
						const libraryPaths = state.pendingFolders > 0 ? state.libraryPaths : configuredLibraryPaths();
						const settings = { ...params.settings, libraryPaths };
						const serialized = JSON.stringify(settings);
						if (serialized === state.lastConfiguration) return;
						await super.sendNotification(type, { ...params, settings });
						if (state === workspaceSync) {
							state.libraryPaths = libraryPaths;
							state.lastConfiguration = serialized;
						}
					});
					state.queue = sent.catch(() => {});
					return sent;
				});
			}
			return super.sendNotification(type, params);
		}
	}
	client = new LugoLanguageClient("lugo", "Lugo LSP", serverOptions, clientOptions);
	const startedClient = client;
	context.subscriptions.push(startedClient.onDidChangeState(event => {
		if (event.newState === State.Stopped) updateWorkspaceStatus({ kind: "end" });
		if (event.newState !== State.Running) return;
		const initializedPaths = initialLibraryPaths;
		// Running is emitted before the initialized notification. start() returns
		// the in-flight startup promise, including feature registration.
		void startedClient.start().then(async () => {
			// The built-in initial folder event is sent during feature registration.
			// Wait for it to be sent before sending a library catch-up.
			await workspaceSync.queue;
			if (client !== startedClient || !startedClient.isRunning() || initializedPaths !== initialLibraryPaths) return;
			const options = buildInitializationOptions();
			const paths = options.libraryPaths;
			if (initialSettings !== JSON.stringify({ ...options, libraryPaths: undefined })) {
				await startedClient.sendNotification("workspace/didChangeConfiguration", { settings: options });
				return;
			}
			if (initializedPaths && JSON.stringify(paths) !== JSON.stringify(workspaceSync.libraryPaths)) {
				await startedClient.sendNotification("workspace/didChangeWorkspaceFolders", { event: { added: [], removed: [] }, libraryPaths: paths });
			}
		}).catch(error => telemetry?.record("workspace_folder_sync_failed", errorMetadata(error)));
	}));

	const startedAt = Date.now();
	try {
		await client.start();
		telemetry?.lspStarted();
		telemetry?.record("lsp_start_succeeded", { duration_ms: elapsedMilliseconds(startedAt) });
	} catch (error) {
		telemetry?.record("lsp_start_failed", { ...errorMetadata(error), duration_ms: elapsedMilliseconds(startedAt) });
		throw error;
	}
}

async function exportDebugData() {
	const startedAt = Date.now();
	try {
		if (!client?.isRunning()) {
			telemetry?.record("debug_export_cancelled", { reason: "client_not_running" });
			vscode.window.showWarningMessage("Lugo LSP is not running yet.");
			return COMMAND_CANCELLED;
		}

		const selected = await vscode.window.showQuickPick(debugExportCategories, {
			canPickMany: true,
			title: "Lugo: Export Debug Data",
			placeHolder: "Select the debug data to export",
			ignoreFocusOut: true,
			matchOnDescription: true,
		});

		if (!selected || selected.length === 0) {
			telemetry?.record("debug_export_cancelled", { reason: "no_categories" });
			return COMMAND_CANCELLED;
		}

		const workspaceName = vscode.workspace.name || "workspace",
			safeName = workspaceName.replace(/[^a-z0-9._-]+/gi, "-").replace(/^-+|-+$/g, "") || "workspace",
			stamp = new Date().toISOString().replace(/[:.]/g, "-");

		const target = await vscode.window.showSaveDialog({
			defaultUri: vscode.Uri.file(path.join(os.homedir(), `${safeName}-lugo-debug-${stamp}.json`)),
			saveLabel: "Export Debug Data",
			filters: {
				"JSON files": ["json"],
				"All files": ["*"],
			},
		});

		if (!target) {
			telemetry?.record("debug_export_cancelled", { reason: "save_dialog" });
			return COMMAND_CANCELLED;
		}

		await vscode.window.withProgress(
			{
				location: vscode.ProgressLocation.Notification,
				title: "Lugo: Exporting debug data...",
				cancellable: false,
			},
			async () => {
				const res = await client.sendRequest("lugo/debugExport", {
					categories: selected.map(item => item.category),
				});

				await vscode.workspace.fs.writeFile(target, new TextEncoder().encode(res.content));
			}
		);

		telemetry?.record("debug_export_succeeded", { duration_ms: elapsedMilliseconds(startedAt), category_count: selected.length });
		const action = await vscode.window.showInformationMessage("Lugo debug data exported successfully.", "Open File");
		if (action === "Open File") {
			const doc = await vscode.workspace.openTextDocument(target);
			await vscode.window.showTextDocument(doc, {preview: false});
		}
	} catch (error) {
		telemetry?.record("debug_export_failed", { ...errorMetadata(error), duration_ms: elapsedMilliseconds(startedAt) });
		vscode.window.showErrorMessage("Lugo debug export failed.");
		throw error;
	}
}

async function triggerReindex() {
	if (!client?.isRunning() || indexing) {
		telemetry?.record("reindex_cancelled", { reason: !client?.isRunning() ? "client_not_running" : "already_indexing" });
		return COMMAND_CANCELLED;
	}

	indexing = true;
	const startedAt = Date.now();
	try {
		await vscode.window.withProgress(
			{
				location: vscode.ProgressLocation.Window,
				title: "Lugo: Indexing workspace...",
				cancellable: false,
			},
			() => client.sendRequest("lugo/reindex")
		);
		telemetry?.record("reindex_succeeded", { duration_ms: elapsedMilliseconds(startedAt) });
	} catch (error) {
		telemetry?.record("reindex_failed", { ...errorMetadata(error), duration_ms: elapsedMilliseconds(startedAt) });
		throw error;
	} finally {
		indexing = false;
	}
}

async function deactivate() {
	if (debounce) clearTimeout(debounce);
	if (!client) {
		telemetry?.record("client_shutdown_succeeded", { had_client: false });
		await telemetry?.shutdown();
		return undefined;
	}

	const startedAt = Date.now();
	telemetry?.record("client_shutdown_started");
	try {
		await client.stop();
		telemetry?.record("client_shutdown_succeeded", { had_client: true, duration_ms: elapsedMilliseconds(startedAt) });
	} catch (error) {
		telemetry?.record("client_shutdown_failed", { ...errorMetadata(error), duration_ms: elapsedMilliseconds(startedAt) });
		throw error;
	} finally {
		await telemetry?.shutdown();
	}
}

module.exports = {
	activate: activate,
	deactivate: deactivate,
};
