package lsp

import "testing"

func TestNewIndexConditionalWritesKeepAlternativeTypes(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local object = setmetatable({}, { __newindex = sink })
if condition then
  object.answer = 42
else
  object.answer = "text"
end
local result = sink.answer
`)
	got := inferMemberForReceiver(t, doc, "sink", "answer")
	if got.Basics&(TypeNumber|TypeString) != TypeNumber|TypeString {
		t.Fatalf("sink.answer = %#v, want number|string as a branch approximation", got)
	}
}
