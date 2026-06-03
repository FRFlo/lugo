const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");

let client, restarting, indexing, debounce;

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
		libraryPaths: resolveLibraryPathsToAbsolute(lugoConfig.get("workspace.libraryPaths") || []),
		ignoreGlobs: ignoreGlobs,
		knownGlobals: lugoConfig.get("environment.knownGlobals") || [],
		bannedSymbols: lugoConfig.get("diagnostics.bannedSymbols") || {},
		maxFileSizeMB: lugoConfig.get("workspace.maxFileSizeMB") ?? 4,
		telemetryEnabled: lugoConfig.get("telemetry.enabled") !== false,

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
 * workspace folder root. Absolute paths are passed through as-is.
 */
function resolveLibraryPathsToAbsolute(globs) {
	const workspaceFolders = vscode.workspace.workspaceFolders || [];
	const resolved = [];

	for (const glob of globs) {
		if (path.isAbsolute(glob)) {
			// Already absolute — use as-is (backward compat)
			resolved.push(glob);
			continue;
		}

		// Workspace-relative glob: resolve against each workspace folder
		const pattern = glob.endsWith("/**") ? glob.slice(0, -3) : glob;

		for (const folder of workspaceFolders) {
			const absPath = path.join(folder.uri.fsPath, pattern);

			try {
				if (fs.existsSync(absPath) && fs.statSync(absPath).isDirectory()) {
					resolved.push(absPath);
				}
			} catch {
				// Skip inaccessible paths silently
			}
		}
	}

	return resolved;
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
		vscode.commands.registerCommand("lugo.addToLibraryPaths", (clickedFile, selectedFiles) => {
			// When triggered from context menu, VS Code passes the URI directly.
			// When multiple files are selected, selectedFiles is an array.
			const targets = selectedFiles && selectedFiles.length > 0 ? selectedFiles : [clickedFile];

			for (const target of targets) {
				addToLibraryPaths(target);
			}
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand("lugo.addToIgnoredGlobs", (clickedFile, selectedFiles) => {
			const targets = selectedFiles && selectedFiles.length > 0 ? selectedFiles : [clickedFile];

			for (const target of targets) {
				addToIgnoredGlobs(target);
			}
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
		run: { command: serverCommand },
		debug: { command: serverCommand },
	};

	const clientOptions = {
		documentSelector: [
			{ scheme: "file", language: "lua" },
			{ scheme: "untitled", language: "lua" },
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
