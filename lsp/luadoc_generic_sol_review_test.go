package lsp

import "testing"

func TestLuaDocGenericSolReviewSubstitutions(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "optional parameter does not widen inferred T",
			source: `---@generic T
---@param value T?
---@return T
local function identity(value) return value end
local result = identity(42)
`,
			want: "number",
		},
		{
			name: "array return",
			source: `---@generic T
---@param value T
---@return T[]
local function wrap(value) return {} end
local result = wrap(42)
`,
			want: "number[]",
		},
		{
			name: "callable return with generic result",
			source: `---@generic T
---@param value T
---@return fun(): T
local function wrap(value) return function() return value end end
local callback = wrap(42)
local result = callback()
`,
			want: "number",
		},
		{
			name: "parameter annotations can differ from prototype order",
			source: `---@generic T
---@param second T
---@param first string
---@return T
local function second(first, second) return second end
local result = second("hello", 42)
`,
			want: "number",
		},
		{
			name: "array parameter binds element type",
			source: `---@generic T
---@param values T[]
---@return T
local function head(values) return values[1] end
---@type number[]
local values = {}
local result = head(values)
`,
			want: "number",
		},
		{
			name: "callable identity preserves callable metadata",
			source: `---@generic T
---@param value T
---@return T
local function identity(value) return value end
---@type fun(): number
local original = function() return 42 end
local callback = identity(original)
local result = callback()
`,
			want: "number",
		},
		{
			name: "fixed callable return preserves callable metadata",
			source: `---@generic T
---@param value T
---@return fun(): number
local function wrap(value) return function() return 42 end end
local callback = wrap("hello")
local result = callback()
`,
			want: "number",
		},
		{
			name: "nested generic return uses bound",
			source: `---@generic T: string
---@return Box<T>
local function make() return {} end
local result = make()
`,
			want: "Box<string>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := newIndexTestDocument(t, tc.source)
			doc.Server = NewServer("sol-review")
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
			if formatted := got.Format(); formatted != tc.want {
				t.Fatalf("inferred result = %q (%+v), want %q", formatted, got, tc.want)
			}
		})
	}
}

func TestLuaDocGenericSolReviewMethodBindings(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "array parameter on method",
			source: `local object = {}
---@generic T
---@param values T[]
---@return T
function object:head(values) return values[1] end
---@type number[]
local values = {}
local result = object:head(values)
`,
			want: "number",
		},
		{
			name: "reordered docs with implicit self",
			source: `local object = {}
---@generic T
---@param second T
---@param self table
---@param first string
---@return T
function object:choose(first, second) return second end
local result = object:choose("hello", 42)
`,
			want: "number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := newIndexTestDocument(t, tc.source)
			doc.Server = NewServer("sol-review")
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
			if formatted := got.Format(); formatted != tc.want {
				t.Fatalf("inferred result = %q (%+v), want %q", formatted, got, tc.want)
			}
		})
	}
}
