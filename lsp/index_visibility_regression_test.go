package lsp

import "testing"

func TestRemoveGlobalIndexDocumentSymbolsKeepsInterleavedCandidate(t *testing.T) {
	idx := NewGlobalIndex()
	key := GlobalKey{PropHash: 17}
	removedURI := "file:///a.lua"
	first := &SymbolEntry{Name: "value", URI: removedURI, NodeID: 1, Key: key}
	survivor := &SymbolEntry{Name: "value", URI: "file:///b.lua", NodeID: 2, Key: key}
	last := &SymbolEntry{Name: "value", URI: "file:///z.lua", NodeID: 3, Key: key, Export: &ExportData{SourceURI: ResourceURI(removedURI)}}
	for _, entry := range []*SymbolEntry{first, survivor, last} {
		idx.AddSymbol("resource", GlobalIndexScopeServer, "value", entry)
	}
	before := idx.LookupCandidatesByScope("resource", GlobalIndexScopeServer, "value")
	if len(before) != 3 || before[0] != first || before[1] != survivor || before[2] != last {
		t.Fatalf("candidate precondition = %+v", before)
	}
	(&Server{GlobalIndex: idx}).removeGlobalIndexDocumentSymbols(removedURI)
	if got := idx.LookupCandidatesByScope("resource", GlobalIndexScopeServer, "value"); len(got) != 1 || got[0] != survivor {
		t.Fatalf("candidates = %+v, want survivor", got)
	}
	if got := idx.SymbolsByHash(key); len(got) != 1 || got[0] != survivor {
		t.Fatalf("hash candidates = %+v, want survivor", got)
	}
	if got := idx.LookupByScope("resource", GlobalIndexScopeServer, "value"); got != survivor {
		t.Fatalf("scope representative = %+v, want survivor", got)
	}
}

func TestFiveMCrossResourceIncludeServerAndUnknownScope(t *testing.T) {
	s := NewServer("include-scope")
	g := NewFiveMResourceGraph()
	s.FiveMResourceGraph = g
	source := &FiveMResourceGraphNode{RootURI: "/source"}
	target := &FiveMResourceGraphNode{Resource: &FiveMResource{RootURI: "/target"}}
	g.ByRoot[source.RootURI] = source
	g.ByRoot[target.Resource.RootURI] = target
	g.ByName["target"] = target
	include := func(path string) FiveMResourceGraphExpansion {
		return FiveMResourceGraphExpansion{Kind: FiveMResourceGraphExpansionInclude, IncludeResource: "target", IncludePath: path}
	}
	source.SharedEntries = []FiveMResourceGraphExpansion{include("shared.lua")}
	source.ServerEntries = []FiveMResourceGraphExpansion{include("server.lua")}
	source.ClientEntries = []FiveMResourceGraphExpansion{include("client.lua")}
	for _, tc := range []struct {
		name string
		kind FiveMExecutionProfileKind
		path string
		want bool
	}{
		{"server shared", FiveMProfileServer, "shared.lua", true},
		{"server server", FiveMProfileServer, "server.lua", true},
		{"server client", FiveMProfileServer, "client.lua", false},
		{"unknown shared", FiveMProfilePlainLua, "shared.lua", true},
		{"unknown server", FiveMProfilePlainLua, "server.lua", false},
		{"unknown client", FiveMProfilePlainLua, "client.lua", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := FiveMExecutionProfile{ResourceRoot: "/source", Kind: tc.kind}
			doc := &Document{URI: "/target/" + tc.path, FiveMProfileCached: true, FiveMProfile: FiveMExecutionProfile{ResourceRoot: "/target"}}
			if got := s.canSeeFiveMCrossResourceInclude(profile, doc); got != tc.want {
				t.Fatalf("visibility = %v, want %v", got, tc.want)
			}
		})
	}
}
