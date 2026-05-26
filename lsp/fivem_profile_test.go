package lsp

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/coalaura/lugo/ast"
)

func TestFiveMProfileMatrix(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)

	plainDoc := addFiveMTestDocument(t, s, filepath.Join(root, "plain.lua"), "return exports, source")
	manifestDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "fxmanifest.lua"), `
client_scripts { 'client.lua', 'dual.lua' }
server_scripts { 'server.lua', 'dual.lua' }
shared_scripts { 'shared.lua' }
`)
	clientDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "client.lua"), "CLIENT_ONLY = true")
	serverDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "server.lua"), "SERVER_ONLY = true")
	sharedDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "shared.lua"), "SHARED_ONLY = true")
	dualDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "dual.lua"), "DUAL_OK = true")

	cases := []struct {
		name string
		doc  *Document
		want FiveMExecutionProfileKind
	}{
		{name: "plain", doc: plainDoc, want: FiveMProfilePlainLua},
		{name: "manifest", doc: manifestDoc, want: FiveMProfileManifest},
		{name: "client", doc: clientDoc, want: FiveMProfileClient},
		{name: "server", doc: serverDoc, want: FiveMProfileServer},
		{name: "shared", doc: sharedDoc, want: FiveMProfileShared},
		{name: "dual listed", doc: dualDoc, want: FiveMProfileShared},
	}

	for _, tc := range cases {
		profile := s.getDocumentFiveMProfile(tc.doc)
		if profile.Kind != tc.want {
			t.Fatalf("%s profile = %s, want %s", tc.name, profile.Kind.String(), tc.want.String())
		}
	}

	bridgeProfile := s.getFiveMExportBridgeProfile(clientDoc)
	if bridgeProfile.Kind != FiveMProfileExportBridge {
		t.Fatalf("client export bridge profile = %s, want %s", bridgeProfile.Kind.String(), FiveMProfileExportBridge.String())
	}

	if got := s.getFiveMExportBridgeProfile(manifestDoc).Kind; got != FiveMProfilePlainLua {
		t.Fatalf("manifest export bridge profile = %s, want %s", got.String(), FiveMProfilePlainLua.String())
	}
}

func TestFiveMProfileIsolation_NonFiveM(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)

	addFiveMTestDocument(t, s, filepath.Join(root, "resource", "fxmanifest.lua"), `server_script 'server.lua'`)
	serverDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "server.lua"), "SERVER_ONLY = true")
	plainDoc := addFiveMTestDocument(t, s, filepath.Join(root, "plain.lua"), "return exports, source")

	if profile := s.getDocumentFiveMProfile(plainDoc); profile.Kind != FiveMProfilePlainLua || profile.ResourceRoot != "" {
		t.Fatalf("plain document profile = %+v, want plain-lua with no resource root", profile)
	}

	if s.isKnownGlobal(plainDoc, []byte("exports")) {
		t.Fatal("plain document unexpectedly exposes exports")
	}

	if s.isKnownGlobal(plainDoc, []byte("source")) {
		t.Fatal("plain document unexpectedly exposes source")
	}

	if s.getFiveMExportBridgeProfile(plainDoc).Kind != FiveMProfilePlainLua {
		t.Fatal("plain document unexpectedly activates the FiveM export bridge")
	}

	if s.canSeeSymbol(plainDoc, serverDoc) {
		t.Fatal("plain document unexpectedly sees FiveM resource symbols")
	}

	for _, name := range []string{"exports", "source"} {
		identID := mustFindIdentNode(t, plainDoc, name)
		if got := plainDoc.InferType(identID); got.Basics != TypeUnknown || got.CustomName != "" {
			t.Fatalf("plain document inferred %s as %+v, want unknown", name, got)
		}
	}
}

func TestFiveMProfileSharedIntersection(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)

	addFiveMTestDocument(t, s, filepath.Join(root, "resource", "fxmanifest.lua"), `
client_scripts { 'client.lua', 'dual.lua', 'dual_consumer.lua' }
server_scripts { 'server.lua', 'dual.lua', 'dual_consumer.lua' }
shared_scripts { 'shared.lua', 'shared_consumer.lua' }
`)
	clientDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "client.lua"), "CLIENT_ONLY = true")
	serverDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "server.lua"), "SERVER_ONLY = true")
	sharedDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "shared.lua"), "SHARED_OK = true")
	dualDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "dual.lua"), "DUAL_OK = true")
	sharedConsumer := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "shared_consumer.lua"), "return CLIENT_ONLY, SERVER_ONLY, SHARED_OK, DUAL_OK, exports, source")
	dualConsumer := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "dual_consumer.lua"), "return CLIENT_ONLY, SERVER_ONLY, SHARED_OK, DUAL_OK, exports, source")

	if !s.canSeeSymbol(clientDoc, sharedDoc) || !s.canSeeSymbol(clientDoc, dualDoc) {
		t.Fatal("client profile should see shared-surface documents")
	}

	if !s.canSeeSymbol(serverDoc, sharedDoc) || !s.canSeeSymbol(serverDoc, dualDoc) {
		t.Fatal("server profile should see shared-surface documents")
	}

	if s.canSeeSymbol(sharedConsumer, clientDoc) || s.canSeeSymbol(sharedConsumer, serverDoc) {
		t.Fatal("shared profile must not see client-only or server-only documents")
	}

	if s.canSeeSymbol(dualConsumer, clientDoc) || s.canSeeSymbol(dualConsumer, serverDoc) {
		t.Fatal("dual-listed profile must stay intersection-only")
	}

	for _, doc := range []*Document{sharedConsumer, dualConsumer} {
		if profile := s.getDocumentFiveMProfile(doc); profile.Kind != FiveMProfileShared {
			t.Fatalf("shared-surface consumer profile = %s, want %s", profile.Kind.String(), FiveMProfileShared.String())
		}

		assertUnresolvedGlobal(t, s, doc, "CLIENT_ONLY")
		assertUnresolvedGlobal(t, s, doc, "SERVER_ONLY")
		assertResolvedGlobal(t, s, doc, "SHARED_OK")
		assertResolvedGlobal(t, s, doc, "DUAL_OK")

		if !s.isKnownGlobal(doc, []byte("exports")) {
			t.Fatal("shared-surface document should keep explicit export bridge access")
		}

		if s.isKnownGlobal(doc, []byte("source")) {
			t.Fatal("shared-surface document must not expose server-only source")
		}
	}
}

func TestFiveMStdlibVisibility(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	indexEmbeddedStdlibForTest(t, s,
		"basic.lua",
		"fivem_client.lua",
		"fivem_server.lua",
		"fivem_shared.lua",
		"io.lua",
		"json.lua",
		"msgpack.lua",
		"os.lua",
		"package.lua",
		"natives_client.lua",
		"natives_server.lua",
		"natives_shared.lua",
	)

	addFiveMTestDocument(t, s, filepath.Join(root, "resource", "fxmanifest.lua"), `
client_script 'client.lua'
server_script 'server.lua'
shared_script 'shared.lua'
`)
	plainDoc := addFiveMTestDocument(t, s, filepath.Join(root, "plain.lua"), "return json, msgpack, package, collectgarbage, TriggerServerEvent, TriggerClientEvent, GlobalState")
	clientDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "client.lua"), "return json, msgpack, package, collectgarbage, TriggerServerEvent, TriggerClientEvent, LocalPlayer, GlobalState")
	serverDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "server.lua"), "return json, msgpack, package, collectgarbage, TriggerServerEvent, TriggerClientEvent, PerformHttpRequest, source, GlobalState")
	sharedDoc := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "shared.lua"), "return json, msgpack, package, collectgarbage, TriggerServerEvent, TriggerClientEvent, GlobalState")

	for _, doc := range []*Document{plainDoc, clientDoc, serverDoc, sharedDoc} {
		assertResolvedGlobal(t, s, doc, "json")
		assertResolvedGlobal(t, s, doc, "msgpack")
		assertResolvedGlobal(t, s, doc, "collectgarbage")
		assertUnresolvedGlobal(t, s, doc, "package")
	}

	assertResolvedGlobal(t, s, plainDoc, "GlobalState")
	assertUnresolvedGlobal(t, s, plainDoc, "TriggerServerEvent")
	assertUnresolvedGlobal(t, s, plainDoc, "TriggerClientEvent")

	for _, doc := range []*Document{clientDoc, serverDoc, sharedDoc} {
		assertResolvedGlobal(t, s, doc, "GlobalState")
	}

	assertResolvedGlobal(t, s, clientDoc, "TriggerServerEvent")
	assertResolvedGlobal(t, s, clientDoc, "LocalPlayer")
	assertUnresolvedGlobal(t, s, clientDoc, "TriggerClientEvent")

	assertResolvedGlobal(t, s, serverDoc, "TriggerClientEvent")
	assertResolvedGlobal(t, s, serverDoc, "PerformHttpRequest")
	assertResolvedGlobal(t, s, serverDoc, "source")
	assertUnresolvedGlobal(t, s, serverDoc, "TriggerServerEvent")

	assertResolvedGlobal(t, s, sharedDoc, "GlobalState")
	assertUnresolvedGlobal(t, s, sharedDoc, "TriggerServerEvent")
	assertUnresolvedGlobal(t, s, sharedDoc, "TriggerClientEvent")
}

func newFiveMProfileTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	root := t.TempDir()
	s := NewServer("test")

	return s, root
}

func addFiveMTestDocument(t *testing.T, s *Server, path, source string) *Document {
	t.Helper()

	uri := s.pathToURI(path)
	s.updateDocument(uri, []byte(source))

	doc := s.Documents[uri]
	if doc == nil {
		t.Fatalf("document %s was not created", uri)
	}

	if doc.IsFiveMManifest {
		res := s.parseFiveMManifest(doc)
		s.registerFiveMManifestResource(res)

		for _, cachedDoc := range s.Documents {
			cachedDoc.FiveMProfile = FiveMExecutionProfile{}
			cachedDoc.FiveMProfileCached = false
		}
	}

	return doc
}

func indexEmbeddedStdlibForTest(t *testing.T, s *Server, names ...string) {
	t.Helper()

	for _, name := range names {
		b, err := stdlibFS.ReadFile("stdlib/" + name)
		if err != nil {
			if name == "package.lua" {
				continue
			}

			t.Fatalf("read embedded stdlib %s: %v", name, err)
		}

		s.updateDocument(embeddedStdlibURIPrefix+name, b)
	}
}

func mustFindIdentNode(t testing.TB, doc *Document, name string) ast.NodeID {
	t.Helper()

	for id := 1; id < len(doc.Tree.Nodes); id++ {
		node := doc.Tree.Nodes[id]
		if node.Kind == ast.KindIdent && bytes.Equal(doc.Source()[node.Start:node.End], []byte(name)) {
			return ast.NodeID(id)
		}
	}

	t.Fatalf("identifier %q not found in %s", name, doc.URI)
	return ast.InvalidNode
}

func assertResolvedGlobal(t *testing.T, s *Server, doc *Document, name string) {
	t.Helper()

	ctx := s.resolveSymbolNode(doc.URI, doc, mustFindIdentNode(t, doc, name))
	if ctx == nil || (ctx.TargetDefID == ast.InvalidNode && len(ctx.GlobalDefs) == 0) {
		t.Fatalf("expected %s to resolve in %s", name, doc.URI)
	}
}

func assertResolvedGlobalTarget(t *testing.T, s *Server, doc *Document, name, wantURI string) {
	t.Helper()

	ctx := s.resolveSymbolNode(doc.URI, doc, mustFindIdentNode(t, doc, name))
	if ctx == nil || (ctx.TargetDefID == ast.InvalidNode && len(ctx.GlobalDefs) == 0) {
		t.Fatalf("expected %s to resolve in %s", name, doc.URI)
	}

	if ctx.TargetURI != wantURI {
		t.Fatalf("%s in %s resolved to %s, want %s", name, doc.URI, ctx.TargetURI, wantURI)
	}
}

func assertUnresolvedGlobal(t *testing.T, s *Server, doc *Document, name string) {
	t.Helper()

	ctx := s.resolveSymbolNode(doc.URI, doc, mustFindIdentNode(t, doc, name))
	if ctx != nil && (ctx.TargetDefID != ast.InvalidNode || len(ctx.GlobalDefs) > 0) {
		t.Fatalf("expected %s to stay hidden in %s", name, doc.URI)
	}
}
