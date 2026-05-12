package lsp

import (
	"slices"
	"testing"
	"time"
)

func TestPrefetchEngine(t *testing.T) {
	graph := NewDependencyGraph()
	graph.SetDependencies("A", []ResourceURI{"B", "C"})
	graph.SetDependencies("B", []ResourceURI{"D"})
	graph.SetDependencies("C", nil)
	graph.SetDependencies("D", nil)

	engine := NewPrefetchEngine(graph, nil)
	engine.Enqueue("A", 2)

	var got []ResourceURI
	engine.IndexFunc = func(uri ResourceURI) {
		got = append(got, uri)
	}

	engine.ProcessQueue(nil)

	want := []ResourceURI{"A", "B", "D", "C"}
	if !slices.Equal(got, want) {
		t.Fatalf("prefetch order = %#v, want %#v", got, want)
	}
}

func TestPrefetchEngineFallbackUpdateDocument(t *testing.T) {
	server := NewServer("test")
	server.Documents["file:///resource"] = &Document{Tree: nil}

	engine := NewPrefetchEngine(nil, nil)
	engine.Enqueue("file:///resource", 0)
	engine.ProcessQueue(server)

	if _, ok := server.Documents["file:///resource"]; !ok {
		t.Fatal("document disappeared during fallback processing")
	}
}

func TestPrefetchEngineDedupesQueuedURIs(t *testing.T) {
	engine := NewPrefetchEngine(nil, nil)
	engine.Enqueue("A", 1)
	engine.Enqueue("A", 1)

	count := 0
	engine.IndexFunc = func(uri ResourceURI) {
		if uri == "A" {
			count++
		}
	}
	engine.ProcessQueue(nil)

	if count != 1 {
		t.Fatalf("queued count = %d, want 1", count)
	}
}

func TestPrefetchEngineEmptyQueue(t *testing.T) {
	engine := NewPrefetchEngine(nil, nil)
	engine.ProcessQueue(nil)
	time.Sleep(10 * time.Millisecond)
}
