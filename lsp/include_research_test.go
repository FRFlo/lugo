package lsp

import (
	"math/rand"
	"testing"
)

func (s *Server) referenceInclude(srcProfile FiveMExecutionProfile, tgtDoc *Document) bool {
	if s == nil || s.FiveMResourceGraph == nil || tgtDoc == nil || srcProfile.ResourceRoot == "" {
		return false
	}

	srcNode := s.getFiveMResourceGraphNode(srcProfile.ResourceRoot)
	if srcNode == nil {
		return false
	}

	tgtProfile := s.getDocumentFiveMProfile(tgtDoc)
	tgtRoot := tgtProfile.ResourceRoot
	if tgtRoot == "" {
		return false
	}

	var relPath string
	if len(tgtDoc.URI) > len(tgtRoot) {
		relPath = tgtDoc.URI[len(tgtRoot)+1:]
	}

	if relPath == "" {
		return false
	}

	matched := false
	srcNode.allowedIncludes(srcProfile.Env(), func(include FiveMResourceGraphExpansion) bool {
		if include.IncludeResource == "" || include.IncludePath == "" {
			return true
		}

		resolved := s.resolveFiveMResource(include.IncludeResource)
		if resolved == nil || resolved.RootURI != tgtRoot {
			return true
		}

		if include.IncludePath == relPath || matchGlob(include.IncludePath, relPath) {
			matched = true
			return false
		}
		return true
	})

	return matched
}

func TestIncludeResearchDifferential(t *testing.T) {
	s := NewServer("mcp")
	g := NewFiveMResourceGraph()
	s.FiveMResourceGraph = g
	src := &FiveMResourceGraphNode{}
	g.ByRoot["/src"] = src
	a := &FiveMResourceGraphNode{Resource: &FiveMResource{RootURI: "/a"}}
	b := &FiveMResourceGraphNode{Resource: &FiveMResource{RootURI: "/b"}}
	doc := &Document{URI: "/a/dir/FILE.lua", FiveMProfileCached: true, FiveMProfile: FiveMExecutionProfile{ResourceRoot: "/a"}}
	rng := rand.New(rand.NewSource(20261116))
	paths := []string{"dir/file.lua", "**/*.lua", "*.lua", "dir/?ILE.lua", "dir\\file.lua", "", "other.lua", "**", "dir/FILE.lua"}
	names := []string{"alias", " ALIAS ", "@alias", "missing", "", "a"}
	for i := 0; i < 20000; i++ {
		// Changes stay within a single server lifetime: positives and negatives must both refresh.
		g.ByName = map[string]*FiveMResourceGraphNode{}
		g.ByProvide = map[string][]*FiveMResourceGraphNode{}
		switch rng.Intn(5) {
		case 0:
			g.ByName["alias"] = a
		case 1:
			g.ByName["alias"] = b
		case 2:
			g.ByProvide["alias"] = []*FiveMResourceGraphNode{a, b}
		case 3:
			g.ByProvide["alias"] = []*FiveMResourceGraphNode{b, a}
		case 4:
			g.ByProvide["alias"] = []*FiveMResourceGraphNode{nil, a}
		}
		a.Resource.RootURI = []string{"/a", "/b"}[rng.Intn(2)]
		src.SharedEntries = nil
		src.ClientEntries = nil
		src.ServerEntries = nil
		for j := 0; j < rng.Intn(15); j++ {
			e := FiveMResourceGraphExpansion{Kind: FiveMResourceGraphExpansionKind(rng.Intn(3)), IncludeResource: names[rng.Intn(len(names))], IncludePath: paths[rng.Intn(len(paths))]}
			switch rng.Intn(3) {
			case 0:
				src.SharedEntries = append(src.SharedEntries, e)
			case 1:
				src.ClientEntries = append(src.ClientEntries, e)
			case 2:
				src.ServerEntries = append(src.ServerEntries, e)
			}
		}
		if i%7 == 0 {
			delete(g.ByRoot, "/src")
		} else {
			g.ByRoot["/src"] = src
		}
		for _, kind := range []FiveMExecutionProfileKind{FiveMProfileClient, FiveMProfileServer, FiveMProfileShared, FiveMProfilePlainLua} {
			p := FiveMExecutionProfile{ResourceRoot: "/src", Kind: kind}
			want := s.referenceInclude(p, doc)
			if got := s.canSeeFiveMCrossResourceInclude(p, doc); got != want {
				t.Fatalf("iteration %d kind %v: got %v want %v", i, kind, got, want)
			}
		}
	}
}
