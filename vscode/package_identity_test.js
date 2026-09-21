const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = __dirname;
const manifest = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));

test('activates for Lua and FiveM resources', () => {
  assert.ok(manifest.activationEvents.includes('onLanguage:lua'));
  assert.ok(manifest.activationEvents.includes('workspaceContains:**/fxmanifest.lua'));
  assert.ok(manifest.activationEvents.includes('workspaceContains:**/__resource.lua'));
});

test('contributes FiveM snippets for Lua files', () => {
  const snippets = manifest.contributes.snippets;
  assert.ok(Array.isArray(snippets));
  assert.ok(snippets.some((entry) => entry.language === 'lua' && entry.path === './snippets/fivem.json'));

  const fivem = JSON.parse(fs.readFileSync(path.join(root, 'snippets', 'fivem.json'), 'utf8'));
  for (const prefix of ['fxmanifest', 'RegisterNetEvent', 'AddEventHandler', 'CreateThread']) {
    assert.ok(Object.values(fivem).some((snippet) => snippet.prefix === prefix), `missing ${prefix} snippet`);
  }
});

test('exposes every FiveM diagnostic and adapter option', () => {
  const properties = Object.assign({}, ...manifest.contributes.configuration.map(group => group.properties));
  for (const key of [
    'lugo.fivem.diagnostics.eventDirection',
    'lugo.fivem.diagnostics.eventPayload',
    'lugo.fivem.diagnostics.unregisteredNetEvent',
    'lugo.fivem.diagnostics.unknownEvent',
    'lugo.fivem.diagnostics.unaccountedFile',
    'lugo.fivem.diagnostics.unknownExport',
    'lugo.fivem.diagnostics.unknownResource',
    'lugo.fivem.diagnostics.trustBoundary',
    'lugo.fivem.diagnostics.performance',
    'lugo.fivem.diagnostics.sql',
    'lugo.fivem.frameworkAdapters',
    'lugo.fivem.sqlAdapters',
  ]) {
    assert.ok(properties[key], `missing ${key}`);
  }

  const extension = fs.readFileSync(path.join(root, 'extension.js'), 'utf8');
  for (const key of ['frameworkAdapters', 'sqlAdapters', 'diagFiveMEventPayload', 'diagFiveMTrustBoundary', 'diagFiveMPerformance', 'diagFiveMSQL']) {
    assert.match(extension, new RegExp(`\\b${key}\\b`), `initialization options omit ${key}`);
  }
});

test('uses the canonical Lugo package and repository identity', () => {
  assert.equal(manifest.name, 'lugo-vscode-fivem-enhanced');
  assert.equal(manifest.publisher, 'FRFlo');
  assert.equal(manifest.repository.url, 'https://github.com/FRFlo/lugo.git');
  assert.equal(`${manifest.publisher}.${manifest.name}`, 'FRFlo.lugo-vscode-fivem-enhanced');
});

function test(name, fn) {
  try {
    fn();
    console.log(`ok - ${name}`);
  } catch (error) {
    console.error(`not ok - ${name}`);
    throw error;
  }
}
