package lsp

import (
	"fmt"
	"sync"
	"testing"
)

func TestNewIndexWriteCandidatesRebuildStringKeysAfterEviction(t *testing.T) {
	const longKey = "a-key-that-is-deliberately-long-to-exercise-cached-string-source-ranges"
	source := fmt.Sprintf("object[\"escaped\\x2dkey\"] = 1\nobject[%q] = 2\nobject[ [[long-bracket-key]] ] = 3\n", longKey)
	doc := benchmarkMetatableDocument(t, []byte(source))
	doc.URI = "file:///string-eviction.lua"
	s := NewServer("string-eviction")
	doc.Server = s
	s.Documents[doc.URI] = doc

	doc.cacheEvictedMetatableNodeText()
	doc.Tree.Source = nil
	s.invalidateMetatableIndexes()

	want := []string{"escaped-key", longKey, "long-bracket-key"}
	var wg sync.WaitGroup
	for _, key := range want {
		key := key
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidates := s.newIndexWriteCandidates(key)
			if len(candidates) != 1 {
				t.Errorf("rebuilt candidates for %q = %d, want 1", key, len(candidates))
			}
		}()
	}
	wg.Wait()
}
