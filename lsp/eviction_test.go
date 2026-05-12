package lsp

import "testing"

func TestEvictionPolicyRegisterEvictionUpdatesExistingEntry(t *testing.T) {
	policy := NewEvictionPolicy(4)
	policy.registerEvictionLocked("res-a", 10)
	policy.registerEvictionLocked("res-b", 20)
	policy.registerEvictionLocked("res-a", 30)

	if got := len(policy.evictionList); got != 2 {
		t.Fatalf("expected 2 eviction entries, got %d", got)
	}

	for _, entry := range policy.evictionList {
		if entry.URI == "res-a" {
			if entry.Timestamp != 30 {
				t.Fatalf("expected updated timestamp 30, got %d", entry.Timestamp)
			}
			return
		}
	}

	t.Fatalf("expected to find res-a in eviction list")
}

func TestEvictionPolicyDoesNotEvictUnderLimit(t *testing.T) {
	policy := NewEvictionPolicy(3)
	idx, docs := newEvictionTestState(t, "res-a", "res-b")

	policy.registerEvictionLocked("res-a", 10)
	policy.registerEvictionLocked("res-b", 20)

	policy.EvictOldest(idx, docs)

	if got := len(policy.evictionList); got != 2 {
		t.Fatalf("expected 2 eviction entries, got %d", got)
	}
	if _, ok := idx.Resources["res-a"]; !ok {
		t.Fatalf("expected res-a to remain in index")
	}
	if _, ok := docs["res-a"]; !ok {
		t.Fatalf("expected res-a to remain in docs")
	}
}

func TestEvictionPolicyEvictsSourcesBeforeDependents(t *testing.T) {
	policy := NewEvictionPolicy(1)
	idx, docs := newEvictionTestState(t, "res-source", "res-dependent")

	source := ResourceURI("res-source")
	dependent := ResourceURI("res-dependent")

	policy.registerEvictionLocked(source, 30)
	policy.registerEvictionLocked(dependent, 10)

	policy.EvictOldest(idx, docs)

	if _, ok := idx.Resources[source]; !ok {
		t.Fatalf("expected source metadata to remain in index")
	}
	if _, ok := docs[string(source)]; ok {
		t.Fatalf("expected source document to be evicted")
	}
	if _, ok := idx.Resources[dependent]; !ok {
		t.Fatalf("expected dependent to remain in index")
	}
	if _, ok := docs[string(dependent)]; !ok {
		t.Fatalf("expected dependent document to remain")
	}
}

func newEvictionTestState(t *testing.T, sourceName, dependentName string) (*GlobalIndex, map[string]*Document) {
	t.Helper()

	idx := NewGlobalIndex()
	docs := map[string]*Document{}

	source := ResourceURI(sourceName)
	dependent := ResourceURI(dependentName)

	idx.Resources[source] = NewResourceScope(source)
	idx.Resources[dependent] = NewResourceScope(dependent)
	idx.DepGraph.AddResource(source)
	idx.DepGraph.SetDependencies(dependent, []ResourceURI{source})
	idx.syncResourceEdgesLocked()

	docs[string(source)] = &Document{URI: string(source)}
	docs[string(dependent)] = &Document{URI: string(dependent)}

	return idx, docs
}
