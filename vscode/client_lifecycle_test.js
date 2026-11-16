const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const source = fs.readFileSync(require.resolve("./extension"), "utf8");
const start = source.slice(source.indexOf("async function startClient("), source.indexOf("async function exportDebugData("));
const resolveSource = source.slice(source.indexOf("function resolveLibraryPathsToAbsolute("), source.indexOf("/**\n * Adds a folder to the library paths"));
const initSource = source.slice(source.indexOf("function buildInitializationOptions("), source.indexOf("function scheduleConfigUpdate("));

{
	const configurations = {
		files: { exclude: { "hidden/**": true, "search-only/**": false, "duplicate/**": true } },
		search: { exclude: { "hidden/**": false, "search-only/**": true, "duplicate/**": true, "disabled/**": false } },
		lugo: { "workspace.ignoreGlobs": ["custom/**", "duplicate/**"] },
	};
	const vscode = { workspace: { getConfiguration: section => ({ get: key => configurations[section][key] }) } };
	const context = { vscode, telemetry: undefined, configuredLibraryPaths: () => [] };
	const build = vm.runInNewContext(`${initSource}\nbuildInitializationOptions`, context);
	assert.deepEqual(Array.from(build().ignoreGlobs), ["custom/**", "duplicate/**", "hidden/**", "search-only/**"]);
	assert.deepEqual(configurations.lugo["workspace.ignoreGlobs"], ["custom/**", "duplicate/**"]);
}

for (const [pathApi, absolute, root] of [
	[require("node:path").posix, "/external/lib", "/"],
	[require("node:path").win32, "C:/external/lib", "C:/"],
]) {
	const context = { path: pathApi, fs: {}, vscode: { workspace: { workspaceFolders: [] } } };
	const resolve = vm.runInNewContext(`${resolveSource}\nresolveLibraryPathsToAbsolute`, context);
	assert.equal(resolve([`${absolute}/**`])[0], absolute);
	assert.equal(resolve([absolute])[0], absolute);
	assert.equal(resolve([`${root}**`])[0], pathApi.parse(root).root);
}

{
	const path = require("node:path").posix;
	const folders = [{ uri: { fsPath: "/a" } }, { uri: { fsPath: "/b" } }];
	const vscode = { workspace: {
		workspaceFolders: folders,
		getConfiguration: (_section, uri) => ({ get: () => uri.fsPath === "/a" ? ["lib/**", "/shared/**"] : ["vendor/**", "/shared/**"] }),
	} };
	const context = { path, fs: { existsSync: () => true, statSync: () => ({ isDirectory: () => true }) }, vscode };
	const configured = vm.runInNewContext(`${resolveSource}\nconfiguredLibraryPaths`, context);
	assert.deepEqual(Array.from(configured()), ["/a/lib", "/shared", "/b/vendor"]);
}

(async () => {
	let options;
	let currentSetting = 1;
	let currentPaths = ["/library"];
	let reindexRequests = 0;
	const notifications = [];
	let onState;
	let startup = Promise.resolve();
	let notificationGate;
	let configurationGate;
	let running = true;
	let failures = 0;
	let attempts = 0;
	class LanguageClient {
		constructor(_id, _name, _server, clientOptions) { options = clientOptions; }
		async start() { await startup; }
		isRunning() { return running; }
		onDidChangeState(listener) { onState = listener; return { dispose() {} }; }
		async sendNotification(method, params) {
			await startup;
			if (notificationGate && (typeof method === "string" ? method : method.method) === "workspace/didChangeWorkspaceFolders") await notificationGate;
			if (configurationGate && (typeof method === "string" ? method : method.method) === "workspace/didChangeConfiguration") await configurationGate;
			attempts++;
			if (failures > 0) { failures--; throw new Error("send failed"); }
			notifications.push({ method, params });
		}
	}
	const context = {
		buildInitializationOptions: () => ({ setting: currentSetting, libraryPaths: currentPaths }),
		configuredLibraryPaths: () => currentPaths,
		os: { platform: () => "linux", arch: () => "x64" },
		path: require("node:path"), fs: { existsSync: () => true },
		vscode: { workspace: { createFileSystemWatcher: () => ({}) } },
		LanguageClient, State: { Running: 2 }, telemetry: undefined, client: undefined,
		elapsedMilliseconds: () => 0,
		triggerReindex: () => { reindexRequests++; },
	};
	vm.runInNewContext(`${start}\nthis.startClient = startClient;`, context);
	await context.startClient({ extensionPath: "/extension", subscriptions: [] });
	assert.equal(reindexRequests, 0, "startup must not request a second reindex");
	assert.equal(typeof options.initializationOptions, "function");
	assert.equal(options.initializationOptions().setting, 1);
	currentSetting = 2;
	assert.equal(options.initializationOptions().setting, 2, "restart must use current settings");
	// WorkspaceFoldersFeature's startup catch-up uses a protocol notification
	// object rather than the middleware, so exercise that actual call shape.
	const protocolType = { method: "workspace/didChangeWorkspaceFolders" };
	await context.client.sendNotification(protocolType, {
		event: { added: [{ uri: "file:///new", name: "new" }], removed: [] },
	});
	assert.equal(notifications.length, 1);
	assert.equal(notifications[0].method, protocolType);
	assert.equal(notifications[0].params.event.added[0].uri, "file:///new");
	assert.deepEqual(Array.from(notifications[0].params.libraryPaths), ["/library"]);
	await context.client.sendNotification("workspace/didChangeWorkspaceFolders", { event: { added: [], removed: [] } });
	assert.deepEqual(Array.from(notifications[1].params.libraryPaths), ["/library"]);
	notifications.pop();
	currentPaths = ["/after-restart"];
	let finishStartup;
	startup = new Promise(resolve => { finishStartup = resolve; });
	onState({ newState: 2 });
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 1, "must wait for initialized and feature registration");
	finishStartup();
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 2);
	assert.deepEqual(Array.from(notifications[1].params.libraryPaths), ["/after-restart"]);
	assert.equal(notifications[1].params.event.added.length, 0);
	// A settings change can arrive after initialize captured the options but
	// before the client is running, when the normal debounced update is dropped.
	options.initializationOptions();
	startup = new Promise(resolve => { finishStartup = resolve; });
	onState({ newState: 2 });
	currentSetting = 3;
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 2, "must not send configuration before startup completes");
	finishStartup();
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 3);
	assert.equal(notifications[2].method, "workspace/didChangeConfiguration");
	assert.equal(notifications[2].params.settings.setting, 3);
	options.initializationOptions();
	onState({ newState: 2 });
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 3, "unchanged settings must not trigger a notification");
	const payload = { settings: { setting: 4 } };
	await context.client.sendNotification("other/notification", payload);
	assert.equal(notifications[3].params, payload, "other notifications must be forwarded unchanged");
	// Running is emitted before feature registration sends its catch-up. Its
	// transport can still be pending after start() resolves; it must reach the
	// server before our paths-only reconciliation.
	options.initializationOptions();
	currentPaths = ["/folder-added-during-startup"];
	startup = new Promise(resolve => { finishStartup = resolve; });
	let finishNotification;
	notificationGate = new Promise(resolve => { finishNotification = resolve; });
	onState({ newState: 2 });
	const initialFolderEvent = context.client.sendNotification(protocolType, {
		event: { added: [{ uri: "file:///folder-added", name: "folder-added" }], removed: [] },
	});
	finishStartup();
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 4, "reconciliation must wait for the initial folder event");
	finishNotification();
	await initialFolderEvent;
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications[4].method, protocolType);
	assert.equal(notifications.length, 5, "initial folder event already carried current libraries");
	options.initializationOptions();
	startup = new Promise(resolve => { finishStartup = resolve; });
	notificationGate = new Promise(resolve => { finishNotification = resolve; });
	onState({ newState: 2 });
	currentSetting = 4;
	const configDuringStartup = context.client.sendNotification("workspace/didChangeConfiguration", { settings: { setting: 4, libraryPaths: ["/old"] } });
	currentPaths = ["/new-folder"];
	const folderDuringStartup = context.client.sendNotification(protocolType, { event: { added: [{ uri: "file:///new-folder", name: "new" }], removed: [] } });
	finishStartup();
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, 5, "configuration must not overtake the initial folder event");
	finishNotification();
	await Promise.all([configDuringStartup, folderDuringStartup]);
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications[5].method, protocolType);
	assert.equal(notifications[6].method, "workspace/didChangeConfiguration");
	assert.deepEqual(Array.from(notifications[6].params.settings.libraryPaths), ["/new-folder"]);
	const afterConfiguration = notifications.length;
	await context.client.sendNotification("workspace/didChangeConfiguration", { settings: { setting: 4, libraryPaths: ["/stale"] } });
	assert.equal(notifications.length, afterConfiguration, "identical effective configuration must be sent only once");
	let finishConfiguration;
	configurationGate = new Promise(resolve => { finishConfiguration = resolve; });
	const beforeInversion = notifications.length;
	const delayedConfig = context.client.sendNotification("workspace/didChangeConfiguration", { settings: { setting: 9, libraryPaths: currentPaths } });
	await new Promise(resolve => setImmediate(resolve));
	currentPaths = ["/later-folder"];
	const laterFolder = context.client.sendNotification(protocolType, { event: { added: [{ uri: "file:///later", name: "later" }], removed: [] } });
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, beforeInversion, "folder notification must wait for configuration already being sent");
	finishConfiguration();
	await Promise.all([delayedConfig, laterFolder]);
	assert.equal(notifications[beforeInversion].method, "workspace/didChangeConfiguration");
	assert.deepEqual(Array.from(notifications[beforeInversion].params.settings.libraryPaths), ["/new-folder"]);
	assert.equal(notifications[beforeInversion + 1].method, protocolType);
	assert.deepEqual(Array.from(notifications[beforeInversion + 1].params.libraryPaths), ["/later-folder"]);
	configurationGate = undefined;
	currentPaths = ["/new-folder"];
	const beforeRestoringPaths = notifications.length;
	await context.client.sendNotification("workspace/didChangeConfiguration", { settings: { setting: 9, libraryPaths: currentPaths } });
	assert.equal(notifications.length, beforeRestoringPaths + 1, "a folder update must not hide a later library-path change");
	configurationGate = new Promise(resolve => { finishConfiguration = resolve; });
	const beforeQueuedChanges = notifications.length;
	const blockingConfig = context.client.sendNotification("workspace/didChangeConfiguration", { settings: { setting: 10, libraryPaths: currentPaths } });
	await new Promise(resolve => setImmediate(resolve));
	const queuedConfig = context.client.sendNotification("workspace/didChangeConfiguration", { settings: { setting: 11, libraryPaths: currentPaths } });
	await new Promise(resolve => setImmediate(resolve));
	currentPaths = ["/third-folder"];
	const queuedFolder = context.client.sendNotification(protocolType, { event: { added: [{ uri: "file:///third", name: "third" }], removed: [] } });
	finishConfiguration();
	await Promise.all([blockingConfig, queuedConfig, queuedFolder]);
	assert.deepEqual(Array.from(notifications[beforeQueuedChanges + 1].params.settings.libraryPaths), ["/new-folder"], "queued config must retain the server's latest library paths");
	assert.deepEqual(Array.from(notifications[beforeQueuedChanges + 2].params.libraryPaths), ["/third-folder"]);
	configurationGate = undefined;
	let beforeAttempts = attempts;
	failures = 1;
	await context.client.sendNotification(protocolType, { event: { added: [], removed: [] } });
	assert.equal(attempts - beforeAttempts, 2, "a transient folder notification failure must be retried");
	beforeAttempts = attempts;
	running = false;
	failures = 1;
	await assert.rejects(context.client.sendNotification(protocolType, { event: { added: [], removed: [] } }));
	assert.equal(attempts - beforeAttempts, 1, "a stopped client must not retry folder notifications");
	running = true;
	let unblockOld;
	notificationGate = new Promise(resolve => { unblockOld = resolve; });
	const oldInFlight = context.client.sendNotification(protocolType, { event: { added: [], removed: [] } });
	await new Promise(resolve => setImmediate(resolve));
	const oldQueued = context.client.sendNotification(protocolType, { event: { added: [], removed: [] } });
	options.initializationOptions(); // a restarted server has a fresh folder snapshot
	notificationGate = undefined;
	currentPaths = ["/after-another-restart"];
	const beforeRestart = notifications.length;
	onState({ newState: 2 });
	await new Promise(resolve => setImmediate(resolve));
	assert.equal(notifications.length, beforeRestart + 1, "an old pending notification must not block the new generation");
	unblockOld();
	await Promise.all([oldInFlight, oldQueued]);
	assert.equal(notifications.length, beforeRestart + 2, "queued notifications from an old generation must be discarded");
	console.log("ok - startup indexes once and restart reads current settings");
})().catch(error => { console.error(error); process.exitCode = 1; });
