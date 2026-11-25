// The stable VS Code API does not expose the native expandable-hover controls.
// Reopen the hover at the requested depth using a command restricted to Lugo.
function createTableHoverController(vscode, getClient) {
	let pending;

	async function provideHover(document, position, token, next) {
		const client = getClient();
		if (!client?.isRunning()) return next(document, position, token);
		const uri = document.uri.toString();
		const matches = pending && pending.uri === uri && pending.version === document.version &&
			pending.line === position.line && pending.character === position.character;
		const depth = matches ? pending.depth : 0;
		pending = undefined;
		const params = client.code2ProtocolConverter.asTextDocumentPositionParams(document, position);
		params.lugoTableDepth = depth;
		let result;
		try {
			result = await client.sendRequest("textDocument/hover", params, token);
		} catch (error) {
			if (token.isCancellationRequested) return undefined;
			return next(document, position, token);
		}
		if (!result || token.isCancellationRequested) return undefined;
		const hover = client.protocol2CodeConverter.asHover(result);
		if (!result.lugoTable) return hover;
		const links = [];
		const link = (label, targetDepth) => {
			const args = encodeURIComponent(JSON.stringify([uri, document.version, position.line, position.character, targetDepth]));
			return `[${label}](command:lugo.tableHoverDepth?${args})`;
		};
		if (result.lugoTable.depth > 0) links.push(link("− Less detail", result.lugoTable.depth - 1));
		if (result.lugoTable.more) links.push(link("+ More detail", result.lugoTable.depth + 1));
		if (links.length) {
			const controls = new vscode.MarkdownString(links.join("　"));
			controls.isTrusted = { enabledCommands: ["lugo.tableHoverDepth"] };
			hover.contents.push(controls);
		}
		return hover;
	}

	async function changeDepth(uri, version, line, character, depth) {
		if (typeof uri !== "string" || ![version, line, character, depth].every(Number.isSafeInteger) ||
			line < 0 || character < 0 || depth < 0) return;
		const editor = vscode.window.visibleTextEditors.find(item => item.document.uri.toString() === uri && item.document.version === version);
		if (!editor) return;
		const position = new vscode.Position(line, character);
		if (!editor.document.validatePosition(position).isEqual(position)) return;
		await vscode.window.showTextDocument(editor.document, { viewColumn: editor.viewColumn });
		pending = { uri, version, line, character, depth };
		editor.selection = new vscode.Selection(position, position);
		await vscode.commands.executeCommand("editor.action.hideHover");
		await vscode.commands.executeCommand("editor.action.showHover", { focus: "autoFocusImmediately" });
	}

	return { provideHover, changeDepth };
}

module.exports = { createTableHoverController };
