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

test('uses the canonical Lugo package and repository identity', () => {
  assert.equal(manifest.name, 'lugo-vscode');
  assert.equal(manifest.publisher, 'coalaura');
  assert.equal(manifest.repository.url, 'https://github.com/coalaura/lugo.git');
  assert.equal(`${manifest.publisher}.${manifest.name}`, 'coalaura.lugo-vscode');
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
