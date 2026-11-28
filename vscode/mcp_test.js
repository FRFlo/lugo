const assert = require("node:assert/strict");
const fs = require("node:fs");
const { registerMcp } = require("./mcp");
const exists = fs.existsSync;
fs.existsSync = () => true;
try {
	const folder = { name: "project", uri: { scheme: "file", fsPath: "/project" } };
	let provider, workspaceChanged;
	const workspace = {
		workspaceFolders: [folder],
		getConfiguration: () => ({ get: () => false }),
		onDidChangeWorkspaceFolders: fn => (workspaceChanged = fn, { dispose() {} }),
		onDidChangeConfiguration: () => ({ dispose() {} }),
	};
	class EventEmitter { constructor() { this.event = () => {}; } fire() {} dispose() {} }
	class Definition { constructor(label, command, args, env) { Object.assign(this, { label, command, args, env }); } }
	const context = { extensionPath: "/extension", subscriptions: [] };
	registerMcp({ workspace, EventEmitter, McpStdioServerDefinition: Definition, lm: { registerMcpServerDefinitionProvider: (id, value) => (assert.equal(id, "lugo"), provider = value, { dispose() {} }) } }, context);
	const definitions = provider.provideMcpServerDefinitions();
	assert.equal(definitions.length, 1);
	assert.deepEqual(definitions[0].args, ["--mcp", "/project"]);
	assert.equal(definitions[0].env.LUGO_MCP_TELEMETRY, "0");
	const registered = [], removed = [];
	registerMcp({ workspace, cursor: { mcp: { registerServer: server => registered.push(server), unregisterServer: name => removed.push(name) } } }, { extensionPath: "/extension", subscriptions: [] });
	assert.deepEqual(registered[0].server.args, ["--mcp", "/project"]);
	workspace.workspaceFolders = [];
	workspaceChanged();
	assert.deepEqual(removed, [registered[0].name]);
} finally { fs.existsSync = exists; }
