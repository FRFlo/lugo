const assert = require("node:assert/strict");
const { createTableHoverController } = require("./hover_table");

(async () => {
	class Position {
		constructor(line, character) { this.line = line; this.character = character; }
		isEqual(other) { return this.line === other.line && this.character === other.character; }
	}
	class Selection { constructor(start, end) { this.start = start; this.end = end; } }
	class MarkdownString { constructor(value) { this.value = value; } }
	const document = { uri: { toString: () => "file:///test.lua" }, version: 2, validatePosition: p => p };
	const editor = { document, viewColumn: 1 };
	const commands = [];
	const vscode = {
		Position, Selection, MarkdownString,
		window: { visibleTextEditors: [editor], showTextDocument: async () => editor },
		commands: { executeCommand: async command => commands.push(command) },
	};
	let requested;
	const client = {
		isRunning: () => true,
		code2ProtocolConverter: { asTextDocumentPositionParams: () => ({ textDocument: { uri: document.uri.toString() }, position: { line: 1, character: 2 } }) },
		protocol2CodeConverter: { asHover: result => ({ contents: [result.contents] }) },
		sendRequest: async (_method, params) => {
			requested = params;
			return { contents: "hover", lugoTable: { depth: params.lugoTableDepth, more: params.lugoTableDepth < 2 } };
		},
	};
	const controller = createTableHoverController(vscode, () => client);
	const position = new Position(1, 2);
	const token = { isCancellationRequested: false };
	let hover = await controller.provideHover(document, position, token, () => {});
	assert.equal(requested.lugoTableDepth, 0);
	assert.match(hover.contents[1].value, /More detail/);
	assert.deepEqual(hover.contents[1].isTrusted.enabledCommands, ["lugo.tableHoverDepth"]);
	const encodedArgs = hover.contents[1].value.match(/command:lugo\.tableHoverDepth\?([^)]*)/)[1];
	assert.deepEqual(JSON.parse(decodeURIComponent(encodedArgs)), [document.uri.toString(), 2, 1, 2, 1]);
	await controller.changeDepth(document.uri.toString(), 2, 1, 2, 1);
	assert.deepEqual(commands, ["editor.action.hideHover", "editor.action.showHover"]);
	hover = await controller.provideHover(document, position, token, () => {});
	assert.equal(requested.lugoTableDepth, 1);
	assert.match(hover.contents[1].value, /Less detail/);
	assert.match(hover.contents[1].value, /More detail/);
	await controller.changeDepth(document.uri.toString(), 2, 1, 2, 2);
	hover = await controller.provideHover(document, position, token, () => {});
	assert.match(hover.contents[1].value, /Less detail/);
	assert.doesNotMatch(hover.contents[1].value, /More detail/);
	await controller.changeDepth(document.uri.toString(), 1, 1, 2, 2);
	await controller.provideHover(document, position, token, () => {});
	assert.equal(requested.lugoTableDepth, 0, "a stale document must not change depth");
	client.sendRequest = async () => { throw new Error("request failed"); };
	assert.equal(await controller.provideHover(document, position, token, () => "fallback"), "fallback");
	assert.equal(await controller.provideHover(document, position, { isCancellationRequested: true }, () => "fallback"), undefined);
	console.log("table hover tests passed");
})().catch(error => { console.error(error); process.exitCode = 1; });
