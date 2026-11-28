const assert = require("node:assert/strict");
const { createTableHoverProvider } = require("./hover_table");

(async () => {
	class VerboseHover {
		constructor(contents, range, canIncreaseVerbosity, canDecreaseVerbosity) {
			Object.assign(this, { contents, range, canIncreaseVerbosity, canDecreaseVerbosity });
		}
	}
	let expandTypes = false;
	const vscode = {
		VerboseHover,
		workspace: { getConfiguration: () => ({ get: () => expandTypes }) },
	};
	const document = { uri: "file:///test.lua" };
	const position = { line: 1, character: 2 };
	const token = { isCancellationRequested: false };
	let requested;
	const client = {
		isRunning: () => true,
		code2ProtocolConverter: { asTextDocumentPositionParams: () => ({ textDocument: { uri: document.uri }, position }) },
		protocol2CodeConverter: { asHover: result => ({ contents: [result.contents] }) },
		sendRequest: async (_method, params) => {
			requested = params.lugoTableDepth;
			return {
				contents: "hover",
				lugoTable: { depth: requested, more: requested >= 0 && requested < 2 },
			};
		},
	};
	const provider = createTableHoverProvider(vscode, () => client);
	const hover = context => provider.provideHover(document, position, token, context);
	let result = await hover();
	assert.equal(requested, 0);
	assert.equal(result.canIncreaseVerbosity, true);
	assert.equal(result.canDecreaseVerbosity, false);
	result = await hover({ previousHover: result, verbosityDelta: 1 });
	assert.equal(requested, 1);
	assert.equal(result.canIncreaseVerbosity, true);
	assert.equal(result.canDecreaseVerbosity, true);
	result = await hover({ previousHover: result, verbosityDelta: 1 });
	assert.equal(requested, 2);
	assert.equal(result.canIncreaseVerbosity, false);
	result = await hover({ previousHover: result, verbosityDelta: -1 });
	assert.equal(requested, 1);
	await hover();
	assert.equal(requested, 0);

	expandTypes = true;
	result = await hover();
	assert.equal(requested, -1);
	assert.equal(result.canIncreaseVerbosity, false);
	assert.equal(result.canDecreaseVerbosity, true);
	await hover({ previousHover: result, verbosityDelta: -1 });
	assert.equal(requested, 0);

	client.sendRequest = async () => ({ contents: "plain hover" });
	result = await hover();
	assert.equal(result.canIncreaseVerbosity, false);
	assert.equal(result.canDecreaseVerbosity, false);
	assert.equal(await provider.provideHover(document, position, { isCancellationRequested: true }), undefined);
	console.log("table hover tests passed");
})().catch(error => { console.error(error); process.exitCode = 1; });
