const fs = require("node:fs");
const path = require("node:path");

function parseManifest(text, manifestPath) {
	const resourceName = firstString(text, /\bname\s*['"]([^'"]+)['"]/) || path.basename(path.dirname(manifestPath));
	const hasClient = /\bclient_script\s*['"]/.test(text) || /\bclient_scripts\s*\{/.test(text);
	const hasServer = /\bserver_script\s*['"]/.test(text) || /\bserver_scripts\s*\{/.test(text);
	const hasShared = /\bshared_script\s*['"]/.test(text) || /\bshared_scripts\s*\{/.test(text);
	const profile = hasShared || (hasClient && hasServer) ? "shared" : hasServer ? "server" : hasClient ? "client" : "unknown";
	const dependencies = [...text.matchAll(/\bdepend(?:ency|encies)\s*(?:['"]([^'"]+)['"]|\{([^}]*)\})/g)]
		.flatMap(match => match[1] ? [match[1]] : [...(match[2] || "").matchAll(/['"]([^'"]+)['"]/g)].map(item => item[1]))
		.filter((value, index, values) => values.indexOf(value) === index)
		.sort();
	return { name: resourceName, profile, dependencies, manifestPath, diagnostics: 0 };
}

function firstString(text, pattern) {
	const match = text.match(pattern);
	return match && match[1];
}

function findManifests(root, results = []) {
	let entries;
	try { entries = fs.readdirSync(root, { withFileTypes: true }); } catch { return results; }
	for (const entry of entries) {
		if (entry.name === "node_modules" || entry.name === ".git") continue;
		const fullPath = path.join(root, entry.name);
		if (entry.isDirectory()) findManifests(fullPath, results);
		else if (entry.name === "fxmanifest.lua" || entry.name === "__resource.lua") results.push(fullPath);
	}
	return results;
}

function discoverResources(workspaceFolders, diagnostics = []) {
	const resources = [];
	for (const folder of workspaceFolders || []) {
		for (const manifestPath of findManifests(folder)) {
			try {
				const resource = parseManifest(fs.readFileSync(manifestPath, "utf8"), manifestPath);
				const root = path.dirname(manifestPath) + path.sep;
				resource.diagnostics = diagnostics.filter(item => item.path === manifestPath || item.path.startsWith(root)).reduce((total, item) => total + item.count, 0);
				resources.push(resource);
			} catch { /* Ignore unreadable manifests. */ }
		}
	}
	return resources.sort((a, b) => a.name.localeCompare(b.name) || a.manifestPath.localeCompare(b.manifestPath));
}

function renderResource(resource) {
	const dependencies = resource.dependencies.length ? resource.dependencies.join(", ") : "none";
	return `${resource.name} · ${resource.profile} · deps: ${dependencies} · diagnostics: ${resource.diagnostics}`;
}

module.exports = { parseManifest, discoverResources, renderResource, findManifests };
