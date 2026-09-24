const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const source = fs.readFileSync(require.resolve("./extension"), "utf8");
const telemetrySource = source.slice(source.indexOf("const DEFAULT_LIMIT"), source.indexOf("class FiveMResourceItem"));
const telemetryModule = { exports: {} };
vm.runInNewContext(`${telemetrySource}\ntelemetryModule.exports = { Telemetry, redact, errorMetadata };`, { crypto: require("node:crypto"), process, telemetryModule });
const { Telemetry, redact, errorMetadata } = telemetryModule.exports;

function makeStorage() {
	const values = new Map();
	return { get: (key, fallback) => values.has(key) ? values.get(key) : fallback, update: (key, value) => values.set(key, value), values };
}
async function test(name, fn) {
	try { await fn(); console.log(`ok - ${name}`); }
	catch (error) { console.error(`not ok - ${name}`, error); process.exitCode = 1; }
}

test("redacts paths, source, and MCP arguments", () => {
	const result = redact({ path: "C:\\Users\\me\\project\\main.lua", source: "local secret = true", args: ["--workspace", "/private/project"] });
	assert.equal(result.path, "[REDACTED]");
	assert.equal(result.source, "[REDACTED]");
	assert.equal(result.args, "[REDACTED]");
});

test("reports only bounded error identities", () => {
	const error = new Error("failed reading C:\\private\\secret.lua: local password = 'x'");
	error.name = "RequestError";
	error.code = "E_FAILURE";
	assert.deepEqual(JSON.parse(JSON.stringify(errorMetadata(error))), { error_type: "RequestError", error_code: "E_FAILURE" });
	assert.deepEqual(JSON.parse(JSON.stringify(errorMetadata({ name: "../../secret", code: "C:\\private" }))), { error_type: "Error" });
});

test("does not capture or persist when opted out", () => {
	const posthog = { capture() { throw new Error("must not send"); } };
	const storage = makeStorage();
	const telemetry = new Telemetry({ posthog, storage, enabled: false, id: () => "session" });
	telemetry.record("ignored", { source: "code" });
	assert.equal(storage.values.get("lugo.telemetry.buffer.v1"), undefined);
	telemetry.setEnabled(true);
	telemetry.record("sent");
	telemetry.setEnabled(false);
	assert.deepEqual(Array.from(storage.values.get("lugo.telemetry.buffer.v1")), []);
});

test("correlates restarts and crash context", () => {
	const events = [];
	const telemetry = new Telemetry({ posthog: { capture: event => events.push(event) }, id: () => "session" });
	telemetry.lspStarted();
	telemetry.lspError(new Error("boom"), "textDocument/hover", 1);
	telemetry.lspRestart("closed");
	telemetry.lspCrash(new Error("crashed"));
	assert.equal(events.at(-1).event, "lsp_crash");
	assert.equal(events.at(-1).properties.session_id, "session");
	assert.equal(events.at(-1).properties.restart_count, 1);
	assert.deepEqual(Array.from(events.at(-1).properties.recent_events), ["lsp_started", "lsp_error", "lsp_restart"]);
});

test("automatic discovery and indexing failures use bounded telemetry events", () => {
	assert.match(source, /resource_discovery_skipped.*reason: "no_resources_found"/);
	assert.match(source, /resource_discovery_failed", errorMetadata\(error\)/);
	assert.match(source, /library_path_unavailable", \{ reason: "inaccessible", \.\.\.errorMetadata\(error\) \}/);
	assert.match(source, /resource_refresh_automatic_failed/);
	assert.match(source, /reindex_automatic_failed/);
	assert.doesNotMatch(source, /resource_discovery_skipped", \{[^}]*path:/);
});

test("keeps the in-memory and persistent buffer bounded", () => {
	const storage = makeStorage();
	const telemetry = new Telemetry({ storage, limit: 3, id: () => "session" });
	for (let i = 0; i < 10; i++) telemetry.record(`event_${i}`);
	assert.equal(telemetry.buffer.length, 3);
	assert.deepEqual(Array.from(telemetry.buffer, item => item.event), ["event_7", "event_8", "event_9"]);
	assert.equal(storage.values.get("lugo.telemetry.buffer.v1").length, 3);
});
