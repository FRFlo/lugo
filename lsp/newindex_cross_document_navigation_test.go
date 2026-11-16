package lsp

import (
	"path/filepath"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestNewIndexCrossDocumentDefinitionFindsUniqueVisibleWrite(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("write.lua", `object.answer = "remote"
`)
	h.writeWorkspaceFile("read.lua", `local result = sink.--[[@remote_newindex_def]]answer
`)
	h.reindex()

	ctx := h.resolve("remote_newindex_def")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("cross-document __newindex field did not resolve: %#v", ctx)
	}
	writeURI := h.server.pathToURI(filepath.Join(h.root, "write.lua"))
	if ctx.TargetDoc != h.server.Documents[writeURI] {
		t.Fatalf("definition document = %v, want write.lua", ctx.TargetDoc)
	}
	if got := string(ctx.TargetDoc.nodeSource(ctx.TargetDefID)); got != "answer" {
		t.Fatalf("definition text = %q, want answer", got)
	}
}

func TestNewIndexCrossDocumentDefinitionIsConservativeForMultipleSources(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("write_a.lua", `object.answer = "a"
`)
	h.writeWorkspaceFile("write_b.lua", `object.answer = "b"
`)
	h.writeWorkspaceFile("read.lua", `local result = sink.--[[@ambiguous_newindex_def]]answer
`)
	h.reindex()

	if ctx := h.resolve("ambiguous_newindex_def"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("ambiguous cross-document writes resolved to one source: %#v", ctx)
	}
}

func TestNewIndexCrossDocumentDefinitionRespectsFiveMVisibility(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource_a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'shared.lua', 'client.lua' }\nserver_scripts { 'server.lua' }\n")
	h.writeWorkspaceFile("resource_a/shared.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("resource_a/client.lua", `local result = sink.--[[@visible_client_newindex]]answer
`)
	h.writeWorkspaceFile("resource_a/server.lua", `object.answer = "server"
`)
	h.writeWorkspaceFile("resource_b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'shared.lua', 'client.lua' }\n")
	h.writeWorkspaceFile("resource_b/shared.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("resource_b/client.lua", `object.answer = "other resource"
`)
	h.reindex()

	if ctx := h.resolve("visible_client_newindex"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("invisible resource/profile write resolved: %#v", ctx)
	}
}
