const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const { parseManifest, renderResource, discoverResources } = require("./fivem_resources");

async function test(name, fn) {
	try { await fn(); console.log(`ok - ${name}`); }
	catch (error) { console.error(`not ok - ${name}`); process.exitCode = 1; }
}

test("registers the FiveM resource view and refresh command", () => {
	const manifest = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
	assert.ok(manifest.contributes.commands.some(command => command.command === "lugo.fivem.refresh"));
	assert.ok(manifest.contributes.views.explorer.some(view => view.id === "lugo.fivemResources"));
});

test("parses resource metadata deterministically", () => {
	const resource = parseManifest(`fx_version 'cerulean'\nname 'city-core'\nshared_script 'shared.lua'\ndependency 'mysql-async'\ndependencies { 'ox_lib', 'mysql-async' }`, "/tmp/city-core/fxmanifest.lua");
	assert.deepEqual(resource, {
		name: "city-core", profile: "shared", dependencies: ["mysql-async", "ox_lib"],
		manifestPath: "/tmp/city-core/fxmanifest.lua", diagnostics: 0
	});
});

test("renders a stable tree/status detail line", () => {
	assert.equal(renderResource({ name: "jobs", profile: "server", dependencies: [], diagnostics: 2 }), "jobs · server · deps: none · diagnostics: 2");
});

test("falls back to the manifest directory name", () => {
	assert.equal(parseManifest("server_script 'server.lua'", "/workspace/resources/chat/__resource.lua").name, "chat");
});

test("discovers resources asynchronously and reports each incrementally", async () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "lugo-fivem-"));
	try {
		fs.mkdirSync(path.join(root, "alpha"));
		fs.mkdirSync(path.join(root, "beta"));
		fs.writeFileSync(path.join(root, "alpha", "fxmanifest.lua"), "client_script 'client.lua'");
		fs.writeFileSync(path.join(root, "beta", "fxmanifest.lua"), "server_script 'server.lua'");
		const discovered = [];
		const resources = await discoverResources([root], [], resource => discovered.push(resource.name));
		assert.deepEqual(resources.map(resource => resource.name), ["alpha", "beta"]);
		assert.deepEqual(discovered, ["alpha", "beta"]);
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});
