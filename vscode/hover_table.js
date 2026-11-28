function createTableHoverProvider(vscode, getClient) {
	const depths = new WeakMap();

	return {
		async provideHover(document, position, token, context) {
			const client = getClient();
			if (!client?.isRunning()) return undefined;

			const previousDepth = depths.get(context?.previousHover);
			const expandTypes = vscode.workspace.getConfiguration("lugo", document.uri).get("hover.expandTypes") === true;
			const depth = previousDepth === undefined
				? (expandTypes ? -1 : 0)
				: Math.max(0, previousDepth + (context.verbosityDelta ?? 0));
			const params = client.code2ProtocolConverter.asTextDocumentPositionParams(document, position);
			params.lugoTableDepth = depth;

			const result = await client.sendRequest("textDocument/hover", params, token);
			if (!result || token.isCancellationRequested) return undefined;

			const hover = client.protocol2CodeConverter.asHover(result);
			const detail = result.lugoTable;
			const verboseHover = new vscode.VerboseHover(
				hover.contents,
				hover.range,
				detail?.more === true,
				detail !== undefined && detail.depth !== 0,
			);
			if (detail) depths.set(verboseHover, detail.depth);
			return verboseHover;
		},
	};
}

module.exports = { createTableHoverProvider };
