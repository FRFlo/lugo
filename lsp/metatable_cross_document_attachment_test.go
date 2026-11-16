package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestMetatableSeparateAttachmentAcrossDocuments(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `object = {}
setmetatable(object, { __index = { answer = 42 } })
`)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@separate_meta_read]]answer
`)
	h.reindex()
	doc := h.docForMarker("separate_meta_read")
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&TypeNumber == 0 {
		t.Fatalf("separate cross-document attachment: got %#v, want number", got)
	}
	ctx := h.resolve("separate_meta_read")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode || ctx.TargetURI == doc.URI {
		t.Fatalf("separate cross-document member did not resolve for navigation: %#v", ctx)
	}
}

func TestMetatableAttachmentLinksGlobalDefinitionAndSetupAcrossDocuments(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("objects.lua", "object = {}\n")
	h.writeWorkspaceFile("setup.lua", `setmetatable(object, { __index = { answer = 42 } })
`)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@three_file_attachment]]answer
`)
	h.reindex()
	got := inferLastNamedMember(t, h.docForMarker("three_file_attachment"), "answer")
	if got.Basics&TypeNumber == 0 {
		t.Fatalf("three-file attachment inference = %#v, want number", got)
	}
}

func TestMetatableAttachmentAcrossDocumentsDoesNotAssumeFileIterationOrder(t *testing.T) {
	for _, files := range []struct {
		name, objectFile, setupFile, readFile string
	}{
		{name: "objects before setup", objectFile: "a_objects.lua", setupFile: "b_setup.lua", readFile: "c_read.lua"},
		{name: "setup before objects", objectFile: "z_objects.lua", setupFile: "a_setup.lua", readFile: "m_read.lua"},
	} {
		t.Run(files.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile(files.objectFile, "object = {}\n")
			h.writeWorkspaceFile(files.setupFile, "setmetatable(object, { __index = { answer = 42 } })\n")
			h.writeWorkspaceFile(files.readFile, "local result = object.--[[@cross_file_order]]answer\n")
			h.reindex()
			if got := inferLastNamedMember(t, h.docForMarker("cross_file_order"), "answer"); got.Basics&TypeNumber == 0 {
				t.Fatalf("file order changed attachment inference = %#v, want number", got)
			}
		})
	}
}

func TestMetatableCrossDocumentAttachmentRebuildsAfterSourceEviction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("objects.lua", "object = {}\n")
	h.writeWorkspaceFile("setup.lua", "setmetatable(object, { __index = { answer = 42 } })\n")
	h.writeWorkspaceFile("read.lua", "local result = object.--[[@evicted_three_file_attachment]]answer\n")
	h.reindex()
	readDoc := h.docForMarker("evicted_three_file_attachment")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("before eviction = %#v, want number", got)
	}

	evictClosedDocumentCaches(h.server)
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("after eviction = %#v, want number", got)
	}
}

func TestMetatableCrossDocumentAttachmentRejectsConflictingSetupFiles(t *testing.T) {
	for _, action := range []string{"edit", "remove"} {
		t.Run(action, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("objects.lua", "object = {}\n")
			h.writeWorkspaceFile("setup_a.lua", "setmetatable(object, { __index = { answer = 42 } })\n")
			h.writeWorkspaceFile("read.lua", "local result = object.--[[@conflicting_setup_files]]answer\n")
			h.reindex()
			readDoc := h.docForMarker("conflicting_setup_files")
			if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
				t.Fatalf("single setup inference = %#v, want number", got)
			}

			setupURI := h.server.pathToURI(filepath.Join(h.root, "setup_b.lua"))
			h.server.updateDocument(setupURI, []byte("setmetatable(object, { __index = { answer = \"other\" } })\n"))
			if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics != TypeUnknown {
				t.Fatalf("conflicting setup inference = %#v, want unknown", got)
			}

			if action == "remove" {
				h.server.clearDocument(setupURI)
			} else {
				h.server.updateDocument(setupURI, nil)
			}
			if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 || got.Basics&TypeString != 0 {
				t.Fatalf("after %s conflicting setup = %#v, want number only", action, got)
			}
		})
	}
}

func TestMetatableGlobalNameAttachmentPreservesOrderAndBranchUncertainty(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		want  BasicType
	}{
		{
			name: "last top-level attachment wins",
			setup: `setmetatable(object, { __index = { answer = "old" } })
setmetatable(object, { __index = { answer = 42 } })
`,
			want: TypeNumber,
		},
		{
			name: "conditional attachment is unknown after branch",
			setup: `setmetatable(object, { __index = { answer = 42 } })
if condition then
  setmetatable(object, { __index = { answer = "branch" } })
end
`,
			want: TypeUnknown,
		},
		{
			name: "unreachable branch is ignored",
			setup: `setmetatable(object, { __index = { answer = 42 } })
if false then
  setmetatable(object, { __index = { answer = "unreachable" } })
end
`,
			want: TypeNumber,
		},
		{
			name: "function body does not replace top-level state",
			setup: `setmetatable(object, { __index = { answer = 42 } })
local function maybeOverride()
  setmetatable(object, { __index = { answer = "function" } })
end
`,
			want: TypeNumber,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("objects.lua", "object = {}\n")
			h.writeWorkspaceFile("setup.lua", tc.setup)
			h.writeWorkspaceFile("read.lua", "local result = object.--[[@global_name_order]]answer\n")
			h.reindex()
			got := inferLastNamedMember(t, h.docForMarker("global_name_order"), "answer")
			if tc.want == TypeUnknown && got.Basics != TypeUnknown || tc.want != TypeUnknown && got.Basics&tc.want == 0 {
				t.Fatalf("global-name attachment inference = %#v, want type %#x", got, tc.want)
			}
		})
	}
}

func TestMetatableCrossDocumentAttachmentObeysFiveMVisibility(t *testing.T) {
	for _, tc := range []struct {
		name          string
		includeObject bool
		wantType      BasicType
	}{
		{name: "both provider files included", includeObject: true, wantType: TypeNumber},
		{name: "object definition hidden", includeObject: false, wantType: TypeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'objects.lua', 'setup.lua' }\n")
			h.writeWorkspaceFile("provider/objects.lua", "object = {}\n")
			h.writeWorkspaceFile("provider/setup.lua", "setmetatable(object, { __index = { answer = 42 } })\n")
			h.writeWorkspaceFile("stranger/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'setup.lua'\n")
			h.writeWorkspaceFile("stranger/setup.lua", "setmetatable(object, { __index = { answer = \"invisible\" } })\n")
			includes := "'@provider/setup.lua'"
			if tc.includeObject {
				includes = "'@provider/setup.lua', '@provider/objects.lua'"
			}
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'read.lua', "+includes+" }\n")
			h.writeWorkspaceFile("consumer/read.lua", "local result = object.--[[@five_m_cross_file_attachment]]answer\n")
			h.reindex()
			got := inferLastNamedMember(t, h.docForMarker("five_m_cross_file_attachment"), "answer")
			if tc.wantType == TypeUnknown && got.Basics != TypeUnknown || tc.wantType != TypeUnknown && got.Basics&tc.wantType == 0 {
				t.Fatalf("FiveM cross-file attachment = %#v, want type %#x", got, tc.wantType)
			}
		})
	}
}

func TestMetatableSeparateAttachmentCacheInvalidatesWhenSetupChanges(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `object = {}
setmetatable(object, { __index = { answer = 42 } })
`)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@cached_separate_meta_read]]answer
`)
	h.reindex()
	readDoc := h.docForMarker("cached_separate_meta_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("initial cross-document attachment = %#v, want number", got)
	}
	setupURI := h.server.pathToURI(filepath.Join(h.root, "setup.lua"))
	h.server.updateDocument(setupURI, []byte(`object = {}
setmetatable(object, { __index = { answer = "changed" } })
`))
	got := inferLastNamedMember(t, readDoc, "answer")
	if got.Basics&TypeString == 0 || got.Basics&TypeNumber != 0 {
		t.Fatalf("updated cross-document attachment = %#v, want string without stale number", got)
	}
	if err := os.Remove(filepath.Join(h.root, "setup.lua")); err != nil {
		t.Fatal(err)
	}
	h.reindex()
	got = inferLastNamedMember(t, readDoc, "answer")
	if got.Basics&(TypeString|TypeNumber) != 0 {
		t.Fatalf("deleted cross-document attachment = %#v, want no stale primitive type", got)
	}
	if _, retained := h.server.metatableTypeDependents[setupURI]; retained {
		t.Fatalf("deleted source retained dependency edges: %#v", h.server.metatableTypeDependents[setupURI])
	}
	for sourceURI, dependents := range h.server.metatableTypeDependents {
		if _, retained := dependents[setupURI]; retained {
			t.Fatalf("deleted source %s remains a dependent of %s", setupURI, sourceURI)
		}
	}
}

func TestMetatableSeparateAttachmentSurvivesSourceEviction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `object = {}
setmetatable(object, { __index = { answer = 42 } })
`)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@evicted_meta_read]]answer
`)
	h.reindex()
	readDoc := h.docForMarker("evicted_meta_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("before eviction attachment = %#v, want number", got)
	}

	evictClosedDocumentCaches(h.server)
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("after eviction attachment = %#v, want number", got)
	}
}

func TestMetatableCrossDocumentTypeInvalidationPropagatesTransitively(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("base.lua", `base = { answer = 42 }
`)
	h.writeWorkspaceFile("setup.lua", `object = {}
setmetatable(object, { __index = base })
`)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@transitive_meta_read]]answer
`)
	h.reindex()
	readDoc := h.docForMarker("transitive_meta_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("initial transitive attachment = %#v, want number", got)
	}
	baseURI := h.server.pathToURI(filepath.Join(h.root, "base.lua"))
	h.server.updateDocument(baseURI, []byte("base = { answer = \"changed\" }\n"))
	got := inferLastNamedMember(t, readDoc, "answer")
	if got.Basics&TypeString == 0 || got.Basics&TypeNumber != 0 {
		t.Fatalf("transitively updated attachment = %#v, want string without stale number", got)
	}
}

func TestMetatableTypeInvalidationHandlesDependencyCycles(t *testing.T) {
	server := NewServer("test")
	for _, uri := range []string{"a", "b", "c"} {
		server.Documents[uri] = &Document{TypeCache: []TypeSet{{Basics: TypeNumber}}, Inferring: []bool{true}}
	}
	server.trackMetatableTypeDependency("a", "b")
	server.trackMetatableTypeDependency("b", "c")
	server.trackMetatableTypeDependency("c", "a")
	server.invalidateMetatableIndexes("a")
	for _, uri := range []string{"a", "b", "c"} {
		if got := server.Documents[uri].TypeCache[0].Basics; got != TypeUnknown {
			t.Fatalf("cache for %s after cyclic invalidation = %#x, want unknown", uri, got)
		}
	}
}

func BenchmarkMetatableTypeDependencyIndexRetained(b *testing.B) {
	for _, dependencies := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("dependencies_%d", dependencies), func(b *testing.B) {
			server := NewServer("dependency-memory")
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for i := 0; i < dependencies; i++ {
				server.trackMetatableTypeDependency("file:///source.lua", fmt.Sprintf("file:///read-%d.lua", i))
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(server)
			retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			if retained < 0 {
				retained = 0
			}
			b.ReportMetric(float64(retained), "retained-B")
			b.ReportMetric(float64(dependencies), "dependencies")
		})
	}
}

func TestMetatableCrossDocumentAttachmentOrderAndUncertainty(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup string
		want  BasicType
	}{
		{
			name: "last attachment wins",
			setup: `object = {}
setmetatable(object, { __index = { answer = "old" } })
setmetatable(object, { __index = { answer = 42 } })
`,
			want: TypeNumber,
		},
		{
			name: "conditional attachment is uncertain after the branch",
			setup: `object = {}
if condition then
  setmetatable(object, { __index = { answer = 42 } })
end
`,
			want: TypeUnknown,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("setup.lua", test.setup)
			h.writeWorkspaceFile("read.lua", `local result = object.--[[@cross_doc_order_read]]answer
`)
			h.reindex()
			got := inferLastNamedMember(t, h.docForMarker("cross_doc_order_read"), "answer")
			if test.want == TypeUnknown && got.Basics != TypeUnknown {
				t.Fatalf("cross-document attachment: got %#v, want unknown", got)
			}
			if test.want != TypeUnknown && got.Basics&test.want == 0 {
				t.Fatalf("cross-document attachment: got %#v, want basics %#x", got, test.want)
			}
		})
	}
}

func TestMetatableCrossDocumentAttachmentCycleTerminates(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `object = {}
base = {}
setmetatable(object, { __index = base })
setmetatable(base, { __index = object })
`)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@cross_doc_cycle_read]]missing
`)
	h.reindex()
	got := inferLastNamedMember(t, h.docForMarker("cross_doc_cycle_read"), "missing")
	if got.Basics&TypeNumber != 0 {
		t.Fatalf("cyclic cross-document attachment: got %#v, want no numeric field", got)
	}
}

func TestMetatableCrossDocumentAttachmentRespectsFiveMVisibility(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("producer/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'setup.lua'
`)
	h.writeWorkspaceFile("producer/setup.lua", `object = {}
setmetatable(object, { __index = { answer = 42 } })
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'read.lua'
`)
	h.writeWorkspaceFile("consumer/read.lua", `local result = object.--[[@cross_doc_fivem_read]]answer
`)
	h.reindex()
	got := inferLastNamedMember(t, h.docForMarker("cross_doc_fivem_read"), "answer")
	if got.Basics&TypeNumber != 0 {
		t.Fatalf("cross-resource attachment leaked into consumer: got %#v", got)
	}
}
