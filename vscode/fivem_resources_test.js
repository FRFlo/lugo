const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const vm = require("node:vm");
const { parseManifest, renderResource, discoverResources } = require("./fivem_resources");

async function test(name, fn) {
	try { await fn(); console.log(`ok - ${name}`); }
	catch (error) { console.error(`not ok - ${name}`); process.exitCode = 1; }
}

test("registers the FiveM resource view and refresh command", () => {
	const manifest = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
	assert.ok(manifest.contributes.commands.some(command => command.command === "lugo.fivem.refresh"));
	assert.ok(manifest.contributes.views.explorer.some(view => view.id === "lugo.fivemResources"));
});

test("exposes the tree change event rather than its emitter", () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const provider = source.match(/class FiveMResourceProvider \{([\s\S]*?)\n\}/)?.[1];
	assert.ok(provider, "FiveMResourceProvider must be defined");
	assert.match(provider, /this\.onDidChangeTreeData\s*=\s*this\._onDidChangeTreeData\.event/);
	assert.doesNotMatch(provider, /this\.onDidChangeTreeData\s*=\s*new vscode\.EventEmitter/);
});

test("parses resource metadata deterministically", () => {
	const resource = parseManifest(`fx_version 'cerulean'\nname 'city-core'\nshared_script 'shared.lua'\ndependency 'mysql-async'\ndependencies { 'ox_lib', 'mysql-async' }`, "/tmp/city-core/fxmanifest.lua");
	assert.deepEqual(resource, {
		name: "city-core", profile: "shared", dependencies: ["mysql-async", "ox_lib"],
		manifestPath: "/tmp/city-core/fxmanifest.lua", diagnostics: 0
	});
});

test("renders a stable tree/status detail line", () => {
	assert.equal(renderResource({ name: "jobs", profile: "server", dependencies: [], diagnostics: 2 }), "jobs · server · deps: none · diagnostics: 2");
});

test("shows workspace loading in the resource view and status bar until diagnostics finish", () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const body = source.match(/updateWorkspaceStatus = progress => \{([\s\S]*?)\n\t\};/)?.[1];
	assert.ok(body);
	const resourceView = {};
	const workspaceStatus = { show() { this.visible = true; }, hide() { this.visible = false; } };
	const update = vm.runInNewContext(`progress => {${body}\n}`, { resourceView, workspaceStatus });
	update({ kind: "begin" });
	assert.equal(workspaceStatus.visible, true);
	assert.match(resourceView.description, /Loading/);
	update({ kind: "report", message: "Analyzing diagnostics: 25/100 files" });
	assert.match(workspaceStatus.text, /25\/100/);
	assert.match(resourceView.description, /25\/100/);
	update({ kind: "end" });
	assert.equal(workspaceStatus.visible, false);
	assert.equal(resourceView.description, undefined);
});

test("falls back to the manifest directory name", () => {
	assert.equal(parseManifest("server_script 'server.lua'", "/workspace/resources/chat/__resource.lua").name, "chat");
});

test("discovers resources asynchronously and reports each incrementally", async () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "lugo-fivem-"));
	try {
		fs.mkdirSync(path.join(root, "alpha"));
		fs.mkdirSync(path.join(root, "beta"));
		fs.writeFileSync(path.join(root, "alpha", "fxmanifest.lua"), "client_script 'client.lua'");
		fs.writeFileSync(path.join(root, "beta", "fxmanifest.lua"), "server_script 'server.lua'");
		const discovered = [];
		const resources = await discoverResources([root], [], resource => discovered.push(resource.name));
		assert.deepEqual(resources.map(resource => resource.name), ["alpha", "beta"]);
		assert.deepEqual(discovered, ["alpha", "beta"]);
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});

test("overlapping workspace folders discover each manifest once in either order", async () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "lugo-overlap-"));
	try {
		const resource = path.join(root, "resources", "alpha");
		fs.mkdirSync(resource, { recursive: true });
		fs.writeFileSync(path.join(resource, "fxmanifest.lua"), "client_script 'client.lua'");
		for (const folders of [[root, resource], [resource, root]]) {
			const discovered = [];
			const resources = await discoverResources(folders, [], item => discovered.push(item.manifestPath));
			assert.equal(resources.length, 1);
			assert.equal(discovered.length, 1);
		}
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});

test("symlinked workspace aliases preserve the first resource URI without rescanning", async () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "lugo-alias-"));
	try {
		const workspace = path.join(root, "workspace");
		const nested = path.join(workspace, "resources");
		const resource = path.join(nested, "alpha");
		const alias = path.join(root, "alias");
		fs.mkdirSync(resource, { recursive: true });
		fs.writeFileSync(path.join(resource, "fxmanifest.lua"), "client_script 'client.lua'");
		try { fs.symlinkSync(nested, alias, "dir"); } catch (error) {
			if (process.platform === "win32" && ["EPERM", "EACCES"].includes(error.code)) return;
			throw error;
		}
		for (const [folders, expected] of [
			[[workspace, alias], path.join(resource, "fxmanifest.lua")],
			[[alias, workspace], path.join(alias, "alpha", "fxmanifest.lua")],
		]) {
			const discovered = [];
			const resources = await discoverResources(folders, [], item => discovered.push(item.manifestPath));
			assert.equal(resources.length, 1);
			assert.deepEqual(discovered, [expected]);
			assert.equal(resources[0].manifestPath, expected);
		}
		const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
		const helper = source.slice(source.indexOf("async function deletionMayAffectResources("), source.indexOf("/**\n * Adds a folder to the library paths"));
		const check = vm.runInNewContext(`${helper}\ndeletionMayAffectResources`, { path, fs });
		const resources = await discoverResources([alias, workspace]);
		const folders = [alias, workspace].map(folder => ({ uri: { fsPath: folder } }));
		assert.equal(await check({ files: [{ fsPath: resource }] }, resources, false, folders), true, "physical deletion must refresh an alias-backed resource");
		const secondAlias = path.join(root, "second-alias");
		fs.symlinkSync(nested, secondAlias, "dir");
		folders.push({ uri: { fsPath: secondAlias } });
		assert.equal(await check({ files: [{ fsPath: path.join(secondAlias, "alpha") }] }, resources, false, folders), true, "another alias must also trigger refresh");
		assert.equal(await check({ files: [{ fsPath: path.join(workspace, "unrelated.lua") }] }, resources, true, folders), true, "incomplete discovery must remain conservative");
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});

test("stops a superseded discovery before visiting further resources", async () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "lugo-cancel-"));
	try {
		for (const name of ["alpha", "beta"]) {
			fs.mkdirSync(path.join(root, name));
			fs.writeFileSync(path.join(root, name, "fxmanifest.lua"), "client_script 'client.lua'");
		}
		let cancelled = false;
		const resources = await discoverResources([root], [], () => { cancelled = true; }, () => cancelled);
		assert.equal(resources.length, 1);
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});

test("diagnostics update existing resources without rescanning and stay current during discovery", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const providerSource = source.slice(source.indexOf("class FiveMResourceProvider"), source.indexOf("const posthogClient"));
	let diagnostics = [];
	let scans = 0;
	let finishDiscovery;
	const resource = { name: "alpha", manifestPath: path.join(os.tmpdir(), "alpha", "fxmanifest.lua"), diagnostics: 0 };
	const vscode = {
		EventEmitter: class { constructor() { this.event = () => {}; } fire() {} dispose() {} },
		languages: { getDiagnostics: () => diagnostics },
		workspace: { workspaceFolders: [{ uri: { fsPath: os.tmpdir() } }] },
	};
	const sandbox = {
		vscode, path, telemetry: undefined,
		discoverResources: async (_folders, _diagnostics, onResource) => {
			scans++;
			onResource(resource);
			await new Promise(resolve => { finishDiscovery = resolve; });
			return [resource];
		},
	};
	const Provider = vm.runInNewContext(`${providerSource}\nFiveMResourceProvider`, sandbox);
	const provider = new Provider();
	const pending = provider.refresh();
	diagnostics = [[{ fsPath: path.join(os.tmpdir(), "alpha", "main.lua") }, [{}, {}]]];
	provider.updateDiagnostics();
	assert.equal(resource.diagnostics, 2);
	finishDiscovery();
	await pending;
	assert.equal(resource.diagnostics, 2);
	provider.updateDiagnostics();
	assert.equal(scans, 1);
});

test("manifest changes update only the resource and discard stale reads", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const providerSource = source.slice(source.indexOf("class FiveMResourceProvider"), source.indexOf("const posthogClient"));
	const manifest = path.join(os.tmpdir(), "alpha", "fxmanifest.lua");
	const reads = [];
	const vscode = {
		EventEmitter: class { constructor() { this.event = () => {}; } fire() {} dispose() {} },
		languages: { getDiagnostics: () => [[{ fsPath: path.join(os.tmpdir(), "alpha", "main.lua") }, [{}, {}]]] },
		workspace: { workspaceFolders: [] },
	};
	const sandbox = {
		vscode, path, parseManifest, telemetry: undefined,
		fs: { promises: { readFile: () => new Promise((resolve, reject) => reads.push({ resolve, reject })) } },
		discoverResources: async () => [],
	};
	const Provider = vm.runInNewContext(`${providerSource}\nFiveMResourceProvider`, sandbox);
	const provider = new Provider();
	provider.resources = [parseManifest("name 'old'", manifest)];
	const oldRead = provider.refreshManifest(manifest);
	const newRead = provider.refreshManifest(manifest);
	reads[1].resolve("name 'new'\nserver_script 'main.lua'");
	assert.equal(await newRead, true);
	reads[0].resolve("name 'stale'");
	assert.equal(await oldRead, true);
	assert.equal(provider.resources[0].name, "new");
	assert.equal(provider.resources[0].profile, "server");
	assert.equal(provider.resources[0].diagnostics, 2);
	assert.equal(await provider.refreshManifest(path.join(os.tmpdir(), "unknown", "fxmanifest.lua")), false);
	const failed = provider.refreshManifest(manifest);
	reads[2].reject(new Error("removed"));
	assert.equal(await failed, false);
	const pending = provider.refreshManifest(manifest);
	await provider.refresh();
	reads[3].resolve("name 'outdated'");
	assert.equal(await pending, true);
	assert.equal(provider.resources.length, 0, "late read must not restore a removed resource");
});

test("known manifest additions and removals stay local and reject stale reads", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const providerSource = source.slice(source.indexOf("class FiveMResourceProvider"), source.indexOf("const posthogClient"));
	const folder = path.join(os.tmpdir(), "alpha");
	const manifest = path.join(folder, "fxmanifest.lua");
	const sibling = path.join(folder, "__resource.lua");
	const reads = [];
	const vscode = {
		EventEmitter: class { constructor() { this.event = () => {}; } fire() {} dispose() {} },
		languages: { getDiagnostics: () => [] }, workspace: { workspaceFolders: [] },
	};
	const sandbox = { vscode, path, parseManifest, telemetry: undefined, fs: { promises: { readFile: () => new Promise(resolve => reads.push(resolve)) } } };
	const Provider = vm.runInNewContext(`${providerSource}\nFiveMResourceProvider`, sandbox);
	const provider = new Provider();
	provider.resources = [parseManifest("name 'alpha'", manifest)];
	const added = provider.addManifest(sibling);
	reads.shift()("name 'legacy'");
	assert.equal(await added, true);
	assert.equal(provider.resources.length, 2);
	const changed = provider.refreshManifest(sibling);
	assert.equal(provider.removeManifest(sibling), true);
	reads.shift()("name 'stale'");
	assert.equal(await changed, true);
	assert.equal(provider.resources.length, 1);
	assert.equal(provider.removeManifest(sibling), true, "duplicate delete event should not rescan");
	const recreated = provider.addManifest(sibling);
	assert.equal(provider.removeManifest(sibling), true, "delete must invalidate a pending recreation");
	reads.shift()("name 'stale recreation'");
	assert.equal(await recreated, true);
	assert.equal(provider.resources.length, 1);
	const unknown = path.join(os.tmpdir(), "unknown", "fxmanifest.lua");
	assert.equal(await provider.addManifest(unknown), false);
	const rootManifest = provider.addManifest(unknown, true);
	reads.shift()("name 'root resource'");
	assert.equal(await rootManifest, true);
	assert.equal(provider.resources.length, 2);
});

test("new manifests are recognized locally only at a single workspace root", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const body = source.slice(source.indexOf("\tconst addKnownManifests ="), source.indexOf("\tcontext.subscriptions.push(vscode.workspace.onDidCreateFiles"));
	let folders = [{ uri: { fsPath: path.join(os.tmpdir(), "workspace") } }];
	const roots = [];
	const sandbox = {
		path, vscode: { workspace: { get workspaceFolders() { return folders; } } },
		resourceDiscoveries: 0, resourceDisposed: false,
		resourceProvider: { resources: [], addManifest: async (_path, knownRoot) => { roots.push(knownRoot); return knownRoot; } },
		resourceStatus: { show() {} },
	};
	const add = vm.runInNewContext(`${body}\naddKnownManifests`, sandbox);
	const uri = { fsPath: path.join(folders[0].uri.fsPath, "fxmanifest.lua") };
	assert.equal(await add([uri]), true);
	folders = [...folders, { uri: { fsPath: path.join(os.tmpdir(), "other") } }];
	assert.equal(await add([uri]), false);
	assert.deepEqual(roots, [true, false]);
});

test("a delayed delete notification reconciles a recreated manifest", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const body = source.slice(source.indexOf("\tconst removeKnownManifests ="), source.indexOf("\tcontext.subscriptions.push(vscode.workspace.onDidDeleteFiles"));
	const manifest = path.join(os.tmpdir(), "alpha", "fxmanifest.lua");
	let exists = false;
	let refreshes = 0;
	const resourceProvider = { resources: [{ manifestPath: manifest }], removedManifests: new Set(), removeManifest(name) {
		this.resources = this.resources.filter(resource => resource.manifestPath !== name);
		this.removedManifests.add(name);
	} };
	const sandbox = {
		path, resourceProvider, resourceDiscoveries: 0, resourceStatus: { show() {} },
		fs: { promises: { stat: async () => { if (!exists) throw Object.assign(new Error("missing"), { code: "ENOENT" }); return {}; } } },
		scheduleResourceRefresh: () => { refreshes++; },
	};
	const remove = vm.runInNewContext(`${body}\nremoveKnownManifests`, sandbox);
	assert.equal(remove([{ fsPath: manifest }]), true);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(refreshes, 0);
	exists = true;
	assert.equal(remove([{ fsPath: manifest }]), true);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(refreshes, 1);
});

test("manifest watcher falls back to discovery for unknown files and scans in progress", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const start = source.indexOf("\tconst manifestWatcher =");
	const registration = source.slice(start, source.indexOf("\tvoid refreshResources().catch", start));
	let onChange;
	let targeted = 0;
	let refreshes = 0;
	let handled = true;
	const watcher = { onDidChange: handler => { onChange = handler; return {}; }, onDidCreate: () => ({}), onDidDelete: () => ({}) };
	const sandbox = {
		context: { subscriptions: [] }, vscode: { workspace: { createFileSystemWatcher: () => watcher } },
		resourceDiscoveries: 0, resourceProvider: { refreshManifest: async () => { targeted++; return handled; } },
		scheduleResourceRefresh: () => { refreshes++; },
	};
	vm.runInNewContext(registration, sandbox);
	onChange({ fsPath: "/known/fxmanifest.lua" });
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(targeted, 1);
	assert.equal(refreshes, 0);
	handled = false;
	onChange({ fsPath: "/unknown/fxmanifest.lua" });
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(refreshes, 1);
	sandbox.resourceDiscoveries = 1;
	onChange({ fsPath: "/known/fxmanifest.lua" });
	assert.equal(targeted, 2);
	assert.equal(refreshes, 2);
});

test("automatic refresh requests during discovery coalesce into one follow-up scan", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const body = source.slice(source.indexOf("\tlet resourceDiscoveries = 0;"), source.indexOf("\tcontext.subscriptions.push(resourceProvider"));
	const pending = [];
	const timers = new Map();
	let timerID = 0;
	let scans = 0;
	let statusShows = 0;
	const sandbox = {
		resourceProvider: { refresh: () => { scans++; return new Promise(resolve => pending.push(resolve)); }, updateDiagnostics() {} },
		resourceStatus: { show() { statusShows++; } }, telemetry: undefined, elapsedMilliseconds: () => 0,
		setTimeout: callback => { const id = ++timerID; timers.set(id, callback); return id; },
		clearTimeout: id => timers.delete(id),
	};
	vm.runInNewContext(`${body}\nthis.start = refreshResources; this.schedule = scheduleResourceRefresh; this.dispose = () => { resourceDisposed = true; };`, sandbox);
	const initial = sandbox.start();
	sandbox.schedule(); sandbox.schedule(); sandbox.schedule();
	assert.equal(scans, 1);
	assert.equal(timers.size, 0);
	pending.shift()([]);
	await initial;
	assert.equal(timers.size, 1);
	const timer = [...timers.values()][0];
	timers.clear();
	timer();
	assert.equal(scans, 2);
	pending.shift()([]);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(timers.size, 0);
	const beforeDispose = statusShows;
	const last = sandbox.start();
	sandbox.dispose();
	pending.shift()([]);
	await last;
	assert.equal(statusShows, beforeDispose, "completed discovery must not touch disposed status");
});

test("workspace folder events rescan only when the ordered roots change", () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const body = source.slice(source.indexOf("\tlet resourceWorkspaceFolders ="), source.indexOf("\tconst addKnownManifests ="));
	let onFolders;
	let refreshes = 0;
	const folder = uri => ({ uri: { toString: () => uri } });
	const vscode = { workspace: { workspaceFolders: [folder("file:///a")], onDidChangeWorkspaceFolders: handler => { onFolders = handler; return {}; } } };
	const sandbox = { vscode, context: { subscriptions: [] }, scheduleResourceRefresh: () => { refreshes++; } };
	vm.runInNewContext(body, sandbox);
	onFolders({ added: [], removed: [] });
	vscode.workspace.workspaceFolders = [folder("file:///a")];
	onFolders({ added: [folder("file:///a")], removed: [folder("file:///a")] });
	assert.equal(refreshes, 0);
	vscode.workspace.workspaceFolders.push(folder("file:///b"));
	onFolders({ added: [folder("file:///b")], removed: [] });
	assert.equal(refreshes, 1);
});

test("file creation refreshes discovery only when a manifest or folder may be involved", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const helper = source.slice(source.indexOf("async function creationMayAffectResources("), source.indexOf("/**\n * Adds a folder to the library paths"));
	const vscode = { FileType: { File: 1 }, workspace: { fs: { stat: async uri => {
		if (uri.type === "missing") throw new Error("missing");
		return { type: uri.type };
	} } } };
	const check = vm.runInNewContext(`${helper}\ncreationMayAffectResources`, { vscode, path });
	const event = (name, type) => ({ files: [{ fsPath: path.join(os.tmpdir(), name), type }] });
	assert.equal(await check(event("main.lua", 1)), false);
	assert.equal(await check(event("fxmanifest.lua", 1)), true);
	assert.equal(await check(event("folder", 2)), true);
	assert.equal(await check(event("unknown", "missing")), true);
});

test("deletions only refresh a complete inventory when a resource may be removed", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const helper = source.slice(source.indexOf("async function deletionMayAffectResources("), source.indexOf("/**\n * Adds a folder to the library paths"));
	const check = vm.runInNewContext(`${helper}\ndeletionMayAffectResources`, { path, fs });
	const root = path.join(os.tmpdir(), "lugo-resources");
	const resources = [{ manifestPath: path.join(root, "alpha", "fxmanifest.lua") }];
	const event = name => ({ files: [{ fsPath: path.join(root, name) }] });
	assert.equal(await check(event("alpha/main.lua"), resources, false, []), false);
	assert.equal(await check(event("alpha2"), resources, false, []), false);
	assert.equal(await check(event("alpha"), resources, false, []), true);
	assert.equal(await check(event("alpha/fxmanifest.lua"), resources, false, []), true);
	assert.equal(await check(event("alpha/main.lua"), resources, true, []), true);
});

test("rename events refresh for moved directories and manifests but not ordinary files", async () => {
	const source = fs.readFileSync(path.join(__dirname, "extension.js"), "utf8");
	const helpers = source.slice(source.indexOf("async function creationMayAffectResources("), source.indexOf("/**\n * Adds a folder to the library paths"));
	const registration = source.slice(source.indexOf("\tcontext.subscriptions.push(vscode.workspace.onDidDeleteFiles("), source.indexOf("\tconst manifestWatcher"));
	const root = path.join(os.tmpdir(), "lugo-resources");
	let onRename;
	let refreshes = 0;
	const vscode = { FileType: { File: 1 }, workspace: {
		fs: { stat: async uri => ({ type: uri.type }) },
		onDidDeleteFiles: () => ({}),
		onDidRenameFiles: handler => { onRename = handler; return {}; },
	} };
	const sandbox = { vscode, path, fs, context: { subscriptions: [] }, resourceProvider: { resources: [] }, resourceDiscoveries: 0, scheduleResourceRefresh: () => { refreshes++; } };
	vm.runInNewContext(`${helpers}\n${registration}`, sandbox);
	const rename = (oldName, newName, type) => onRename({ files: [{ oldUri: { fsPath: path.join(root, oldName) }, newUri: { fsPath: path.join(root, newName), type } }] });
	rename("a.lua", "b.lua", 1);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(refreshes, 0);
	rename("folder", "other", 2);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(refreshes, 1);
	rename("a.lua", "fxmanifest.lua", 1);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(refreshes, 2);
});
