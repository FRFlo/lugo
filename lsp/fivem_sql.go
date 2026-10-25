package lsp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// FiveMSQLAdapterMetadata describes an adapter without tying diagnostics to a
// particular adapter release. Names are matched case-insensitively.
type FiveMSQLAdapterMetadata struct {
	Name      string   `json:"name"`
	Calls     []string `json:"calls,omitempty"`
	SyncCalls []string `json:"syncCalls,omitempty"`
}

// Default adapters intentionally contain only stable call shapes. Projects
// can add adapter metadata through future configuration without changing this
// analyzer; unknown/dynamic calls remain ignored.
var defaultFiveMSQLAdapters = []FiveMSQLAdapterMetadata{
	{Name: "oxmysql", Calls: []string{"MySQL.query", "MySQL.insert", "MySQL.update", "MySQL.scalar", "MySQL.prepare", "exports.oxmysql:query", "exports.oxmysql:execute", "exports.oxmysql:fetchScalar"}, SyncCalls: []string{"MySQL.Sync.fetchAll", "MySQL.Sync.fetchScalar", "MySQL.Sync.execute"}},
	{Name: "mysql-async", Calls: []string{"MySQL.Async.fetchAll", "MySQL.Async.fetchScalar", "MySQL.Async.execute"}, SyncCalls: []string{"MySQL.Sync.fetchAll", "MySQL.Sync.fetchScalar", "MySQL.Sync.execute"}},
}

var sqlSchemaRE = regexp.MustCompile(`(?im)^\s*[-\-]{3}@sql\s+(?:table\s+)?([A-Za-z_][\w]*)\s*(?:\(([^)]*)\)|:\s*([^\r\n]*))?`)
var sqlFromRE = regexp.MustCompile(`(?i)\b(?:from|join|update|into)\s+([A-Za-z_][\w]*)`)
var sqlColumnRE = regexp.MustCompile(`(?i)\b([A-Za-z_][\w]*)\.([A-Za-z_][\w]*)\b`)

func (s *Server) buildFiveMSQLDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || doc.Tree == nil {
		return nil
	}
	schemas := sqlSchemas(doc.Source())
	var out []Diagnostic
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		if n.Kind != ast.KindCallExpr && n.Kind != ast.KindMethodCall {
			continue
		}
		name := performanceNodeCallName(doc, id)
		if name == "" && n.Kind == ast.KindMethodCall && n.Right != ast.InvalidNode {
			// Preserve bracketed exports['oxmysql']:query syntax, which is
			// deliberately not treated as a normal member expression.
			right := doc.Tree.Nodes[n.Right]
			name = string(doc.Source()[n.Left:right.End])
		}
		adapter, sync := s.sqlAdapterCall(name)
		if adapter == "" {
			continue
		}
		if sync {
			code := "fivem-sql-sync"
			if doc.FiveMProfile.Kind == FiveMProfileClient {
				code += "-client"
			} else if doc.FiveMProfile.Kind == FiveMProfileServer {
				code += "-server"
			}
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: code, Message: "Synchronous SQL-like call may block the FiveM thread; prefer the asynchronous adapter API."})
		}
		query := callArgument(doc, n, 0)
		if query == ast.InvalidNode || doc.Tree.Nodes[query].Kind != ast.KindString {
			continue
		}
		sql := unquoteLuaString(string(doc.Source()[doc.Tree.Nodes[query].Start:doc.Tree.Nodes[query].End]))
		if args := callArgument(doc, n, 1); args != ast.InvalidNode && doc.Tree.Nodes[args].Kind == ast.KindTableExpr {
			named := sqlNamedPlaceholders(sql)
			if len(named) > 0 {
				bindings := sqlNamedBindings(doc, args)
				var missing []string
				for name := range named {
					if !bindings[name] {
						missing = append(missing, "@"+name)
					}
				}
				if len(missing) > 0 {
					sort.Strings(missing)
					out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-sql-placeholder-count", Message: fmt.Sprintf("SQL query has named placeholders without matching bindings: %s.", strings.Join(missing, ", "))})
				}
			} else {
				want := strings.Count(sql, "?")
				if want != int(doc.Tree.Nodes[args].Count) {
					out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-sql-placeholder-count", Message: fmt.Sprintf("SQL query has %d placeholders but %d arguments were supplied.", want, doc.Tree.Nodes[args].Count)})
				}
			}
		}
		if len(schemas) > 0 {
			out = append(out, sqlSchemaDiagnostics(doc, id, sql, schemas)...)
		}
	}
	return out
}

func sqlNamedPlaceholders(sql string) map[string]bool {
	out := make(map[string]bool)
	for i := 0; i < len(sql); {
		switch {
		case sql[i] == '\'' || sql[i] == '"' || sql[i] == '`':
			quote := sql[i]
			i++
			for i < len(sql) {
				if sql[i] == '\\' && i+1 < len(sql) {
					i += 2
					continue
				}
				if sql[i] == quote {
					if i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case i+1 < len(sql) && sql[i:i+2] == "--":
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case i+1 < len(sql) && sql[i:i+2] == "/*":
			i += 2
			for i+1 < len(sql) && sql[i:i+2] != "*/" {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			}
		case (sql[i] == '@' || sql[i] == ':') && i+1 < len(sql) && isSQLNameStart(sql[i+1]):
			if i == 0 || sql[i-1] != sql[i] {
				start := i + 1
				i = start + 1
				for i < len(sql) && isSQLNamePart(sql[i]) {
					i++
				}
				out[strings.ToLower(sql[start:i])] = true
			} else {
				i++
			}
		default:
			i++
		}
	}
	return out
}

func sqlNamedBindings(doc *Document, tableID ast.NodeID) map[string]bool {
	out := make(map[string]bool)
	if doc == nil || doc.Tree == nil || tableID == ast.InvalidNode || int(tableID) >= len(doc.Tree.Nodes) {
		return out
	}
	table := doc.Tree.Nodes[tableID]
	for i := uint32(0); i < table.Count; i++ {
		extraIndex := table.Extra + i
		if int(extraIndex) >= len(doc.Tree.ExtraList) {
			break
		}
		fieldID := doc.Tree.ExtraList[extraIndex]
		if fieldID == ast.InvalidNode || int(fieldID) >= len(doc.Tree.Nodes) {
			continue
		}
		field := doc.Tree.Nodes[fieldID]
		if (field.Kind != ast.KindRecordField && field.Kind != ast.KindIndexField) || field.Left == ast.InvalidNode || int(field.Left) >= len(doc.Tree.Nodes) {
			continue
		}
		key := doc.Tree.Nodes[field.Left]
		var name string
		switch key.Kind {
		case ast.KindString:
			name = unquoteLuaString(string(doc.Source()[key.Start:key.End]))
		case ast.KindIdent:
			name = string(doc.Source()[key.Start:key.End])
		}
		if len(name) > 1 && (name[0] == '@' || name[0] == ':') {
			name = name[1:]
		}
		if name != "" {
			out[strings.ToLower(name)] = true
		}
	}
	return out
}

func isSQLNameStart(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func isSQLNamePart(b byte) bool {
	return isSQLNameStart(b) || b >= '0' && b <= '9'
}

type sqlSchema struct{ columns map[string]bool }

func sqlSchemas(src []byte) map[string]sqlSchema {
	out := map[string]sqlSchema{}
	for _, m := range sqlSchemaRE.FindAllSubmatch(src, -1) {
		name := strings.ToLower(string(m[1]))
		cols := map[string]bool{}
		columns := string(m[2])
		if columns == "" {
			columns = string(m[3])
		}
		for _, c := range strings.Split(columns, ",") {
			c = strings.TrimSpace(c)
			if i := strings.IndexAny(c, " \t"); i >= 0 {
				c = c[:i]
			}
			if c != "" {
				cols[strings.ToLower(c)] = true
			}
		}
		out[name] = sqlSchema{columns: cols}
	}
	return out
}

func sqlSchemaDiagnostics(doc *Document, id ast.NodeID, sql string, schemas map[string]sqlSchema) []Diagnostic {
	var out []Diagnostic
	for _, m := range sqlFromRE.FindAllStringSubmatch(sql, -1) {
		table := strings.ToLower(m[1])
		if _, ok := schemas[table]; !ok {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-sql-unknown-table", Message: fmt.Sprintf("SQL references undeclared table %q.", m[1])})
		}
	}
	for _, m := range sqlColumnRE.FindAllStringSubmatch(sql, -1) {
		table, col := strings.ToLower(m[1]), strings.ToLower(m[2])
		schema, ok := schemas[table]
		if ok && len(schema.columns) > 0 && !schema.columns[col] {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-sql-unknown-column", Message: fmt.Sprintf("SQL references undeclared column %q on table %q.", m[2], m[1])})
		}
	}
	return out
}

func (s *Server) sqlAdapterCall(name string) (string, bool) {
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
	// Common exports syntax with an arbitrary resource name is still safe when
	// the operation is one of the well-known SQL verbs.
	if strings.HasPrefix(lower, "exports.") || strings.HasPrefix(lower, "exports:") {
		for _, op := range []string{"query", "execute", "fetchscalar", "fetchall", "insert", "update"} {
			if strings.HasSuffix(lower, ":"+op) {
				return "exports", false
			}
		}
	}
	return "", false
}
