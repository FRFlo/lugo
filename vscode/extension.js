const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");

let client, restarting, indexing, debounce;

const fiveMNativeCacheVersion = "v1";
const fiveMNativeCacheFolderName = "fivem-native-bundles";
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
	if (restarting) {
		return;
	}

	restarting = true;

	try {
		if (client) {
			await client.stop();
		}

		await startClient(context);
	} catch {}

	restarting = false;
}

function buildInitializationOptions() {
	const filesConfig = vscode.workspace.getConfiguration("files"),
		searchConfig = vscode.workspace.getConfiguration("search"),
		lugoConfig = vscode.workspace.getConfiguration("lugo");

	let ignoreGlobs = lugoConfig.get("workspace.ignoreGlobs") || [];

	const nativeExcludes = {
		...(filesConfig.get("exclude") || {}),
		...(searchConfig.get("exclude") || {}),
	};

	for (const [key, val] of Object.entries(nativeExcludes)) {
		if (val === true) {
			ignoreGlobs.push(key);
		}
	}

	ignoreGlobs = [...new Set(ignoreGlobs)];

	return {
		libraryPaths: buildLibraryPaths(lugoConfig.get("workspace.libraryPaths") || []),
		ignoreGlobs: ignoreGlobs,
		knownGlobals: lugoConfig.get("environment.knownGlobals") || [],
		bannedSymbols: lugoConfig.get("diagnostics.bannedSymbols") || {},
		maxFileSizeMB: lugoConfig.get("workspace.maxFileSizeMB") ?? 4,

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
		diagFiveMUnregisteredNetEvent: lugoConfig.get("fivem.diagnostics.unregisteredNetEvent") !== false,
		diagFiveMUnknownEvent: lugoConfig.get("fivem.diagnostics.unknownEvent") !== false,
		diagFiveMUnaccountedFile: lugoConfig.get("fivem.diagnostics.unaccountedFile") !== false,
		diagFiveMUnknownExport: lugoConfig.get("fivem.diagnostics.unknownExport") !== false,
		diagFiveMUnknownResource: lugoConfig.get("fivem.diagnostics.unknownResource") !== false,
	};
}

function buildLibraryPaths(paths) {
	const libraryPaths = [...paths];

	const runtimePath = resolveFiveMNativeCacheDir();
	if (!libraryPaths.includes(runtimePath)) {
		libraryPaths.push(runtimePath);
	}

	return libraryPaths;
}

function resolveFiveMNativeCacheDir() {
	let base = "";

	if (process.platform === "win32") {
		base = process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local");
	} else if (process.platform === "darwin") {
		base = path.join(os.homedir(), "Library", "Caches");
	} else {
		base = process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache");
	}

	if (!base) {
		base = os.tmpdir();
	}

	return path.join(base, "lugo", fiveMNativeCacheFolderName, fiveMNativeCacheVersion);
}

function scheduleConfigUpdate() {
	clearTimeout(debounce);

	debounce = setTimeout(() => {
		if (!client?.isRunning()) {
			return;
		}

		client.sendNotification("workspace/didChangeConfiguration", {
			settings: buildInitializationOptions(),
		});
	}, 1000);
}

async function activate(context) {
	const stdProvider = {
		provideTextDocumentContent: uri => {
			if (!client?.isRunning()) {
				return "";
			}

			return client
				.sendRequest("lugo/readStd", {
					uri: uri.toString(),
				})
				.then(res => {
					return res.content;
				});
		},
	};

	context.subscriptions.push(vscode.workspace.registerTextDocumentContentProvider("std", stdProvider));

	context.subscriptions.push(
		vscode.workspace.onDidChangeConfiguration(async e => {
			if (e.affectsConfiguration("lugo") || e.affectsConfiguration("files.exclude") || e.affectsConfiguration("search.exclude")) {
				scheduleConfigUpdate();
			}
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.reindex", () => {
			triggerReindex();
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.applySafeFixesWorkspace", () => {
			vscode.commands.executeCommand("lugo.applySafeFixes");
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.applySafeFixesFile", () => {
			const editor = vscode.window.activeTextEditor;

			if (editor) {
				vscode.commands.executeCommand("lugo.applySafeFixes", editor.document.uri.toString());
			}
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.exportDebugData", () => {
			return exportDebugData();
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.ignoreDiagnostic", async (uriStr, line, rule, isFile) => {
			const editor = vscode.window.activeTextEditor;

			if (!editor || editor.document.uri.fsPath !== vscode.Uri.parse(uriStr).fsPath) {
				return;
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
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.showReferences", (uriStr, position, locations) => {
			const uri = vscode.Uri.parse(uriStr),
				pos = new vscode.Position(position.line, position.character);

			const locs = locations.map(
				loc =>
					new vscode.Location(vscode.Uri.parse(loc.uri), new vscode.Range(loc.range.start.line, loc.range.start.character, loc.range.end.line, loc.range.end.character))
			);

			vscode.commands.executeCommand("editor.action.showReferences", uri, pos, locs);
		})
	);

	await restartClient(context);
}

async function startClient(context) {
	const initializationOptions = buildInitializationOptions();

	const platform = os.platform(),
		arch = os.arch(),
		ext = platform === "win32" ? ".exe" : "",
		binName = `lugo-${platform}-${arch}${ext}`;

	const serverCommand = path.join(context.extensionPath, "bin", binName);

	if (!fs.existsSync(serverCommand)) {
		vscode.window.showErrorMessage(`Lugo LSP binary not found for your platform: ${binName}`);

		return;
	}

	const serverOptions = {
		run: {command: serverCommand},
		debug: {command: serverCommand},
	};

	const clientOptions = {
		documentSelector: [
			{ scheme: "file", language: "lua" },
			{ scheme: "untitled", language: "lua" },
			{ scheme: "std", language: "lua" },
		],
		synchronize: {
			fileEvents: vscode.workspace.createFileSystemWatcher("**/*.lua"),
		},
		initializationOptions: initializationOptions,
	};

	client = new LanguageClient("lugo", "Lugo LSP", serverOptions, clientOptions);

	await client.start();

	triggerReindex();
}

async function exportDebugData() {
	try {
		if (!client?.isRunning()) {
			vscode.window.showWarningMessage("Lugo LSP is not running yet.");
			return;
		}

		const selected = await vscode.window.showQuickPick(debugExportCategories, {
			canPickMany: true,
			title: "Lugo: Export Debug Data",
			placeHolder: "Select the debug data to export",
			ignoreFocusOut: true,
			matchOnDescription: true,
		});

		if (!selected || selected.length === 0) {
			return;
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
			return;
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

		const action = await vscode.window.showInformationMessage(`Lugo debug data exported to ${target.fsPath}`, "Open File");
		if (action === "Open File") {
			const doc = await vscode.workspace.openTextDocument(target);
			await vscode.window.showTextDocument(doc, {preview: false});
		}
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		vscode.window.showErrorMessage(`Lugo debug export failed: ${message}`);
	}
}

function triggerReindex() {
	if (!client || indexing) {
		return;
	}

	indexing = true;

	vscode.window.withProgress(
		{
			location: vscode.ProgressLocation.Window,
			title: "Lugo: Indexing workspace...",
			cancellable: false,
		},
		async () => {
			try {
				await client.sendRequest("lugo/reindex");
			} finally {
				indexing = false;
			}
		}
	);
}

function deactivate() {
	if (debounce) {
		clearTimeout(debounce);
	}

	if (!client) {
		return undefined;
	}

	return client.stop();
}

module.exports = {
	activate: activate,
	deactivate: deactivate,
};
