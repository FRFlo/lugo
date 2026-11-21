package lsp

import "testing"

func TestInferMethodCallReceiverMetatableAttachmentOrder(t *testing.T) {
	s := NewServer("receiver-attachment-order")
	uri := "file:///tmp/receiver-attachment-order.lua"
	s.updateDocument(uri, []byte(`local object = {}
local before = object:answer()
setmetatable(object, { __index = {
  ---@return string
  answer = function(self) return "value" end,
} })
local after = object:answer()
`))
	doc := s.Documents[uri]
	for _, tc := range []struct {
		name string
		want BasicType
	}{
		{"before", TypeUnknown},
		{"after", TypeString},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, tc.name, 0))
			if got.Basics != tc.want {
				t.Fatalf("inferred %s = %#v, want basics %#x", tc.name, got, tc.want)
			}
		})
	}
}
