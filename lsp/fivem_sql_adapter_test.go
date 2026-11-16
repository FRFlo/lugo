package lsp

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// legacySQLAdapterCall preserves the pre-optimization implementation as a
// differential oracle for adapter ordering and Unicode comparison behavior.
func (s *Server) legacySQLAdapterCall(name string) (string, bool) {
	lower := strings.ToLower(name)
	adapters := defaultFiveMSQLAdapters
	if s != nil && len(s.SQLAdapters) > 0 {
		adapters = append(append([]FiveMSQLAdapterMetadata{}, adapters...), s.SQLAdapters...)
	}
	for _, a := range adapters {
		for _, c := range append(append([]string{}, a.Calls...), a.SyncCalls...) {
			if strings.ToLower(c) == lower {
				for _, sync := range a.SyncCalls {
					if strings.EqualFold(sync, c) {
						return a.Name, true
					}
				}
				return a.Name, false
			}
		}
	}
	if strings.HasPrefix(lower, "exports.") || strings.HasPrefix(lower, "exports:") {
		for _, op := range []string{"query", "execute", "fetchscalar", "fetchall", "insert", "update"} {
			if strings.HasSuffix(lower, ":"+op) {
				return "exports", false
			}
		}
	}
	return "", false
}

func TestSQLAdapterCallMatchesLegacy(t *testing.T) {
	names := []string{"", "MySQL.query", "MYSQL.SYNC.FETCHALL", "exports.foo:query", "exports:foo:update", "exports['foo']:query", "exports.foo:query.await", "ſ", "s", "K", "K", "İ", "i", "Σ", "ς", "σ", string([]byte{255}), "unknown"}
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 20000; i++ {
		var s *Server
		if i%9 != 0 {
			s = &Server{}
			for j := 0; j < r.Intn(5); j++ {
				a := FiveMSQLAdapterMetadata{Name: fmt.Sprint(j)}
				if i%7 == 0 {
					a.Name = ""
				}
				for k := 0; k < r.Intn(8); k++ {
					a.Calls = append(a.Calls, names[r.Intn(len(names))])
				}
				for k := 0; k < r.Intn(8); k++ {
					a.SyncCalls = append(a.SyncCalls, names[r.Intn(len(names))])
				}
				s.SQLAdapters = append(s.SQLAdapters, a)
			}
		}
		for _, name := range names {
			wantName, wantSync := s.legacySQLAdapterCall(name)
			gotName, gotSync := s.sqlAdapterCall(name)
			if gotName != wantName || gotSync != wantSync {
				t.Fatalf("iteration %d, name %q: got %q/%v, want %q/%v", i, name, gotName, gotSync, wantName, wantSync)
			}
		}
	}
}
