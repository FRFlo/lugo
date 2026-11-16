package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FRFlo/lugo/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFiveMEventsMatchNearestEncodedURIRoot(t *testing.T) {
	outerRoot := "file:///workspace/space%20%23%25%2Froot"
	nestedRoot := outerRoot + "/nested%20%23%25"
	aliasRoot := "file:///workspace/other-root"
	graph := lsp.NewFiveMResourceGraph()
	outer := &lsp.FiveMResourceGraphNode{Resource: &lsp.FiveMResource{Name: "outer"}, RootURI: outerRoot}
	nested := &lsp.FiveMResourceGraphNode{Resource: &lsp.FiveMResource{Name: "nested"}, RootURI: nestedRoot}
	alias := &lsp.FiveMResourceGraphNode{Resource: &lsp.FiveMResource{Name: "outer"}, RootURI: aliasRoot}
	graph.ByRoot[outerRoot] = outer
	graph.ByRoot[nestedRoot] = nested
	graph.ByRoot[aliasRoot] = alias
	// Multiple aliases may resolve to one node; ByRoot remains the identity
	// used for document ownership. Separate roots with the same resource name
	// must also keep their event rows.
	graph.ByName["nested-alias"] = nested
	graph.ByName["outer-alias"] = alias

	nestedDocURI := nestedRoot + "/client.lua"
	neighborDocURI := outerRoot + "/nested%20%23%25suffix/client.lua"
	aliasDocURI := aliasRoot + "/client.lua"
	workspace := &lsp.Server{
		Documents: map[string]*lsp.Document{
			nestedDocURI:   {URI: nestedDocURI, FiveMEvents: []lsp.FiveMEventInfo{{Name: "nested:event", Kind: lsp.FiveMEventRegisterNet}}},
			neighborDocURI: {URI: neighborDocURI, FiveMEvents: []lsp.FiveMEventInfo{{Name: "outer:event", Kind: lsp.FiveMEventAddHandler}}},
			aliasDocURI:    {URI: aliasDocURI, FiveMEvents: []lsp.FiveMEventInfo{{Name: "outer:event", Kind: lsp.FiveMEventAddHandler}}},
		},
		FiveMResourceGraph: graph,
	}
	s := &server{workspace: workspace}
	result, err := s.fivemEvents(context.Background(), highLevelRequest(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	var events []struct {
		Resource string `json:"resource"`
		Name     string `json:"name"`
		URI      string `json:"uri"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %+v, want three including duplicate alias event", events)
	}
	got := map[string]string{}
	for _, event := range events {
		got[event.URI] = event.Resource + "|" + event.Name
	}
	if want := "nested|nested:event"; got[nestedDocURI] != want {
		t.Errorf("nested event owner = %q, want %q", got[nestedDocURI], want)
	}
	for _, uri := range []string{neighborDocURI, aliasDocURI} {
		if want := "outer|outer:event"; got[uri] != want {
			t.Errorf("alias event at %q = %q, want %q", uri, got[uri], want)
		}
	}
}

func TestFiveMResourceNodeForURIFindsNearestParent(t *testing.T) {
	graph := lsp.NewFiveMResourceGraph()
	root := "file:///workspace/resource"
	node := &lsp.FiveMResourceGraphNode{Resource: &lsp.FiveMResource{Name: "resource"}}
	graph.ByRoot[root] = node
	if got := fiveMResourceNodeForURI(graph, root+"/dir/file.lua"); got != node {
		t.Fatalf("nearest parent node = %p, want %p", got, node)
	}
	if got := fiveMResourceNodeForURI(graph, "file:///workspace/resource-other/file.lua"); got != nil {
		t.Fatalf("prefix neighbor matched resource: %v", got.Resource.Name)
	}
	if got := fiveMResourceNodeForURI(nil, root+"/file.lua"); got != nil {
		t.Fatalf("nil graph returned node %v", got)
	}
}
