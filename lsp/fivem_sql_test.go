package lsp

import "testing"

func TestFiveMSQLDiagnosticsConservative(t *testing.T) {
	tests := []struct {
		name, source string
		codes        []string
	}{
		{"placeholder mismatch", `MySQL.query('select * from users where id = ? and name = ?', {1})`, []string{"fivem-sql-placeholder-count"}},
		{"schema checks are opt in", `MySQL.query('select missing from users')`, nil},
		{"declared schema", `---@sql table users (id, name)
MySQL.query('select users.missing from users')`, []string{"fivem-sql-unknown-column"}},
		{"dynamic query ignored", `local q = 'select * from users'
MySQL.query(q, {1, 2})`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := newFiveMProfileTestServer(t)
			doc := addFiveMTestDocument(t, s, root+"/server.lua", tt.source)
			diags := s.buildFiveMSQLDiagnostics(doc)
			for _, code := range tt.codes {
				if !hasDiagnosticCode(diags, code) {
					t.Fatalf("diagnostics = %#v, missing %s", diags, code)
				}
			}
		})
	}
}

func TestFiveMSQLSyncContext(t *testing.T) {
	for _, profile := range []struct{ name, file, code string }{
		{"client", "client.lua", "fivem-sql-sync-client"},
		{"server", "server.lua", "fivem-sql-sync-server"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			s, root := newFiveMProfileTestServer(t)
			addFiveMTestDocument(t, s, root+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n"+profile.name+"_script '"+profile.file+"'\n")
			doc := addFiveMTestDocument(t, s, root+"/"+profile.file, `local rows = MySQL.Sync.fetchAll('select 1')`)
			if profile.name == "client" {
				doc.FiveMProfile.Kind = FiveMProfileClient
			} else {
				doc.FiveMProfile.Kind = FiveMProfileServer
			}
			if !hasDiagnosticCode(s.buildFiveMSQLDiagnostics(doc), profile.code) {
				t.Fatalf("missing %s diagnostic", profile.code)
			}
		})
	}
}
