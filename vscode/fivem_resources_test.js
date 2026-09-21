const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { parseManifest, renderResource } = require("./fivem_resources");

function test(name, fn) {
	try { fn(); console.log(`ok - ${name}`); }
	catch (error) { console.error(`not ok - ${name}`); throw error; }
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
