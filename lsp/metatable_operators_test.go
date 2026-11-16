package lsp

import (
	"github.com/FRFlo/lugo/ast"
	"testing"
)

func TestMetatableOperatorResultInference(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         BasicType
	}{
		{"ordinary arithmetic", `local result = 1 + 2`, TypeNumber},
		{"ordinary length", `local result = #"hello"`, TypeNumber},
		{"call", `local value = setmetatable({}, { __call = function() return "called" end })
local result = value()`, TypeString},
		{"addition", `local value = setmetatable({}, { __add = function() return "added" end })
local result = value + value`, TypeString},
		{"metamethod assigned later", `local meta = {}
local value = setmetatable({}, meta)
local result = value + value
meta.__add = function() return "late" end`, TypeNumber},
		{"named function", `local function add() return "named" end
local value = setmetatable({}, { __add = add })
local result = value + value`, TypeString},
		{"assigned metamethod", `local meta = {}
meta.__add = function() return "sum" end
local value = setmetatable({}, meta)
local result = value + value`, TypeString},
		{"length", `local value = setmetatable({}, { __len = function() return "length" end })
local result = #value`, TypeString},
		{"right operand", `local left = {}
local right = setmetatable({}, { __add = function() return "right" end })
local result = left + right`, TypeString},
		{"left operand priority", `local left = setmetatable({}, { __add = function() return "left" end })
local right = setmetatable({}, { __add = function() return true end })
local result = left + right`, TypeString},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("operators.lua", tc.source+"\nlocal use = result --[[@operator_result]]\n")
			h.reindex()
			doc := h.docForMarker("operator_result")
			id := findIdentByOccurrence(t, doc.Tree, "result", 1)
			got := doc.InferType(id)
			if got.Basics != tc.want {
				t.Fatalf("result type = %v, want %v", got.Basics, tc.want)
			}
		})
	}
}

func TestMetatableOperatorLookupRespectsSourcePosition(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("operators.lua", `local meta = {}
local value = setmetatable({}, meta)
local before = value + value
meta.__add = function() return "sum" end
local after = value + value
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/operators.lua")]
	var expressions []ast.Node
	for _, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindBinaryExpr {
			expressions = append(expressions, node)
		}
	}
	if len(expressions) != 2 {
		t.Fatalf("binary expressions = %d, want 2", len(expressions))
	}
	for i, expression := range expressions {
		receiver := doc.InferType(expression.Left)
		_, found := doc.inferOperatorMetamethodAt(receiver, "__add", expression.Start)
		if found != (i == 1) {
			t.Errorf("expression %d found metamethod = %v", i, found)
		}
	}
}

func TestMetatableOperatorReplacementFollowsAccessOrder(t *testing.T) {
	doc := newIndexTestDocument(t, `local mt = { __call = function() return "before" end }
local object = setmetatable({}, mt)
local before = object()
mt.__call = function() return 42 end
local after = object()
mt.__call = nil
local cleared = object()
`)
	for _, tc := range []struct {
		name string
		want BasicType
	}{{"before", TypeString}, {"after", TypeNumber}, {"cleared", TypeUnknown}} {
		id := findIdentByOccurrence(t, doc.Tree, tc.name, 0)
		if got := doc.InferType(id); got.Basics != tc.want {
			t.Errorf("%s = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}
