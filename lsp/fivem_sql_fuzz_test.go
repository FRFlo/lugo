package lsp

import "testing"

func FuzzSQLNamedPlaceholders(f *testing.F) {
	for _, seed := range []string{
		"WHERE id = @id",
		"WHERE name = :name AND owner = @owner",
		"SELECT '@ignored', `@alsoIgnored`, value FROM t -- @comment\nWHERE id = @id",
		"SELECT @@version, value FROM t WHERE id = @id",
		"/* @comment */ SELECT value FROM t WHERE id = @id",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, sql string) {
		for name := range sqlNamedPlaceholders(sql) {
			if name == "" {
				t.Fatal("placeholder scanner returned an empty name")
			}
			for i := 0; i < len(name); i++ {
				if !isSQLNamePart(name[i]) || i == 0 && name[i] >= '0' && name[i] <= '9' {
					t.Fatalf("placeholder scanner returned invalid name %q", name)
				}
			}
		}
	})
}
