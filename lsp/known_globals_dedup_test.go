package lsp

import "testing"

func TestKnownGlobalGlobsEquivalentOrderAndDuplicatesDoNotRepublish(t *testing.T) {
	s := NewServer("test")
	opts := defaultInitializationOptions()
	opts.KnownGlobals = []string{"one.*", "two.*", "exact"}
	s.applyInitializationOptions(opts)
	opts.KnownGlobals = []string{"two.*", "one.*", "exact", "two.*"}
	if reindex, republish := s.applyInitializationOptions(opts); reindex || republish {
		t.Fatalf("equivalent globs: reindex=%v republish=%v", reindex, republish)
	}
	if !s.isKnownGlobal(nil, []byte("one.foo")) || !s.isKnownGlobal(nil, []byte("two.bar")) {
		t.Fatal("known globs must remain active")
	}
	opts.KnownGlobals = []string{"one.*", "exact"}
	if reindex, republish := s.applyInitializationOptions(opts); reindex || !republish {
		t.Fatalf("removed glob: reindex=%v republish=%v", reindex, republish)
	}
}
