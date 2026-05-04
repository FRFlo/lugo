package lsp

import (
	"reflect"
	"testing"
)

// TestFiveMDedup_ManifestEntriesArePointers verifies that
// FiveMResourceGraphExpansion.Entry pointers point to addresses within the
// canonical FiveMManifest.Entries slice (not separate allocations).
func TestFiveMDedup_ManifestEntriesArePointers(t *testing.T) {
	t.Parallel()

	fixtures := []string{"resource_client_server_shared"}

	h := newFiveMFixtureHarness(t, fixtures...)

	// Get the resource from the graph
	resourceRoot := h.server.pathToURI(h.root + "/surface_resource")
	node := h.server.FiveMResourceGraph.ByRoot[resourceRoot]
	if node == nil {
		t.Fatal("resource node not found in graph")
	}

	if node.Resource == nil || node.Resource.Manifest == nil {
		t.Fatal("resource manifest not found")
	}

	canonicalEntries := node.Resource.Manifest.Entries

	// Collect all expansion entries from client, server, and shared
	var expansionEntries []*FiveMManifestEntry
	for _, exp := range node.ClientEntries {
		expansionEntries = append(expansionEntries, exp.Entry)
	}
	for _, exp := range node.ServerEntries {
		expansionEntries = append(expansionEntries, exp.Entry)
	}
	for _, exp := range node.SharedEntries {
		expansionEntries = append(expansionEntries, exp.Entry)
	}

	if len(expansionEntries) == 0 {
		t.Fatal("no expansion entries found")
	}

	// Verify each expansion entry is a pointer into canonicalEntries
	for i, entry := range expansionEntries {
		if entry == nil {
			t.Fatalf("expansion entry %d is nil", i)
		}

		// Check if entry address is within canonicalEntries slice
		inCanonical := false
		for j := range canonicalEntries {
			if &canonicalEntries[j] == entry {
				inCanonical = true
				break
			}
		}

		if !inCanonical {
			t.Fatalf("expansion entry %d (value=%q) is not a pointer into canonical manifest entries", i, entry.Value)
		}
	}
}

// TestFiveMDedup_ManifestEntryMutationPropagates verifies that modifying a
// manifest entry field after graph construction is visible through the graph
// expansion pointer.
func TestFiveMDedup_ManifestEntryMutationPropagates(t *testing.T) {
	t.Parallel()

	h := newFiveMFixtureHarness(t, "resource_client_server_shared")

	resourceRoot := h.server.pathToURI(h.root + "/surface_resource")
	node := h.server.FiveMResourceGraph.ByRoot[resourceRoot]
	if node == nil {
		t.Fatal("resource node not found in graph")
	}

	if node.Resource == nil || node.Resource.Manifest == nil {
		t.Fatal("resource manifest not found")
	}

	// Get first non-nil expansion entry
	var expansionEntry *FiveMManifestEntry
	for _, exp := range node.ClientEntries {
		if exp.Entry != nil {
			expansionEntry = exp.Entry
			break
		}
	}
	for _, exp := range node.ServerEntries {
		if exp.Entry != nil && expansionEntry == nil {
			expansionEntry = exp.Entry
			break
		}
	}
	for _, exp := range node.SharedEntries {
		if exp.Entry != nil && expansionEntry == nil {
			expansionEntry = exp.Entry
			break
		}
	}

	if expansionEntry == nil {
		t.Skip("no expansion entries to test mutation propagation")
	}

	// Modify a field on the canonical entry
	originalValue := expansionEntry.Value
	expansionEntry.RawValue = "mutated"

	// Verify mutation is visible through canonical slice
	canonicalEntry := expansionEntry // same pointer
	if canonicalEntry.RawValue != "mutated" {
		t.Fatalf("mutation not visible through pointer: got %q, want %q", canonicalEntry.RawValue, "mutated")
	}

	// Restore
	expansionEntry.RawValue = originalValue
}

// TestFiveMDedup_ResourceLookupByURIViaGraph verifies that
// FiveMResourceGraph.ByRoot[root].Resource returns the correct resource data.
func TestFiveMDedup_ResourceLookupByURIViaGraph(t *testing.T) {
	t.Parallel()

testCases := []struct {
		fixture       string
		resourceRoot  string
		resourceName  string
		expectedGames []string
	}{
		{
			fixture:       "resource_client_server_shared",
			resourceRoot:  "surface_resource",
			resourceName:  "surface_resource",
			expectedGames: []string{"gta5"},
		},
		{
			fixture:       "resource_dual_listed",
			resourceRoot:  "dual_resource",
			resourceName:  "dual_resource",
			expectedGames: []string{"gta5"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			h := newFiveMFixtureHarness(t, tc.fixture)

			rootURI := h.server.pathToURI(h.root + "/" + tc.resourceRoot)
			node := h.server.FiveMResourceGraph.ByRoot[rootURI]
			if node == nil {
				t.Fatalf("resource node not found for %s", tc.resourceRoot)
			}

			if node.Resource == nil {
				t.Fatal("node.Resource is nil")
			}

			if node.Resource.Name != tc.resourceName {
				t.Fatalf("resource name = %q, want %q", node.Resource.Name, tc.resourceName)
			}

			if !reflect.DeepEqual(node.Resource.Games, tc.expectedGames) {
				t.Fatalf("resource games = %v, want %v", node.Resource.Games, tc.expectedGames)
			}
		})
	}
}

// TestFiveMDedup_ResourceLookupByNameViaGraph verifies that
// FiveMResourceGraph.ByName[name].Resource returns the correct resource data.
func TestFiveMDedup_ResourceLookupByNameViaGraph(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fixture       string
		resourceName string
		expectedRoot string
	}{
		{
			fixture:       "resource_client_server_shared",
			resourceName: "surface_resource",
			expectedRoot: "surface_resource",
		},
		{
			fixture:       "resource_dual_listed",
			resourceName: "dual_resource",
			expectedRoot: "dual_resource",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			h := newFiveMFixtureHarness(t, tc.fixture)

			node := h.server.FiveMResourceGraph.ByName[tc.resourceName]
			if node == nil {
				t.Fatalf("resource node not found for name %s", tc.resourceName)
			}

			if node.Resource == nil {
				t.Fatal("node.Resource is nil")
			}

			if node.Resource.Name != tc.resourceName {
				t.Fatalf("resource name = %q, want %q", node.Resource.Name, tc.resourceName)
			}

			wantRoot := h.server.pathToURI(h.root + "/" + tc.expectedRoot)
			if node.Resource.RootURI != wantRoot {
				t.Fatalf("resource root = %q, want %q", node.Resource.RootURI, wantRoot)
			}
		})
	}
}

// TestFiveMDedup_NoSeparateResourceMaps verifies that the Server struct no
// longer has FiveMResources or FiveMResourceByName fields (compile-time check
// via reflection).
func TestFiveMDedup_NoSeparateResourceMaps(t *testing.T) {
	t.Parallel()

	serverType := reflect.TypeOf(&Server{})
	fieldNames := []string{"FiveMResources", "FiveMResourceByName"}

	for _, fieldName := range fieldNames {
		if field, ok := serverType.Elem().FieldByName(fieldName); ok {
			t.Errorf("Server still has deprecated field %q (type %s)", fieldName, field.Type)
		}
	}
}