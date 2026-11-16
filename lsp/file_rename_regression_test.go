package lsp

import (
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestFileRenameRetainsManifestAndFirstCallEdits(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'old.lua'\nrequire('old')\n")
	h.writeWorkspaceFile("consumer.lua", "require('old')\n")
	h.writeWorkspaceFile("old.lua", "return {}\n")
	h.reindex()
	consumerURI := h.server.pathToURI(h.root + "/consumer.lua")
	manifestURI := h.server.pathToURI(h.root + "/fxmanifest.lua")
	doc := h.server.Documents[consumerURI]
	foundFirst := false
	for _, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindCallExpr && node.Count > 0 && node.Extra == 0 {
			foundFirst = true
		}
	}
	if !foundFirst {
		t.Fatal("fixture must exercise a call with Extra == 0")
	}
	edit := h.server.computeRequirePathUpdates([]FileRename{{OldURI: h.server.pathToURI(h.root + "/old.lua"), NewURI: h.server.pathToURI(h.root + "/new.lua")}})
	if got := edit.Changes[consumerURI]; len(got) != 1 || !strings.Contains(got[0].NewText, "new") {
		t.Fatalf("consumer edits = %+v", got)
	}
	if got := edit.Changes[manifestURI]; len(got) != 2 {
		t.Fatalf("manifest needs directive and require edits; got %+v", got)
	}
}
