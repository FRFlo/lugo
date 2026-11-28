const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");

function registerMcp(vscode, context) {
	const ext = os.platform() === "win32" ? ".exe" : "";
	const command = path.join(context.extensionPath, "bin", `lugo-${os.platform()}-${os.arch()}${ext}`);
	if (!fs.existsSync(command)) return;
	const folders = () => (vscode.workspace.workspaceFolders || []).filter(folder => folder.uri.scheme === "file");
	const env = () => ({ LUGO_MCP_TELEMETRY: vscode.workspace.getConfiguration("lugo").get("telemetry.enabled") === false ? "0" : "1" });

	if (vscode.cursor?.mcp) {
		const registered = new Set();
		const refresh = () => {
			for (const name of registered) vscode.cursor.mcp.unregisterServer(name);
			registered.clear();
			for (const folder of folders()) {
				const name = `lugo-${crypto.createHash("sha256").update(folder.uri.fsPath).digest("hex").slice(0, 12)}`;
				vscode.cursor.mcp.registerServer({ name, server: { command, args: ["--mcp", folder.uri.fsPath], env: env() } });
				registered.add(name);
			}
		};
		refresh();
		context.subscriptions.push({ dispose: () => { for (const name of registered) vscode.cursor.mcp.unregisterServer(name); } });
		context.subscriptions.push(vscode.workspace.onDidChangeWorkspaceFolders(refresh));
		context.subscriptions.push(vscode.workspace.onDidChangeConfiguration(event => { if (event.affectsConfiguration("lugo.telemetry.enabled")) refresh(); }));
	} else if (vscode.lm?.registerMcpServerDefinitionProvider && vscode.McpStdioServerDefinition) {
		const changed = new vscode.EventEmitter();
		context.subscriptions.push(changed, vscode.workspace.onDidChangeWorkspaceFolders(() => changed.fire()),
			vscode.workspace.onDidChangeConfiguration(event => { if (event.affectsConfiguration("lugo.telemetry.enabled")) changed.fire(); }),
			vscode.lm.registerMcpServerDefinitionProvider("lugo", {
				onDidChangeMcpServerDefinitions: changed.event,
				provideMcpServerDefinitions: () => folders().map(folder => new vscode.McpStdioServerDefinition(`Lugo: ${folder.name}`, command, ["--mcp", folder.uri.fsPath], env())),
			}));
	}
}

module.exports = { registerMcp };
