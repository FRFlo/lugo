package lsp

import (
	"strings"

	"github.com/FRFlo/lugo/ast"
)

func (doc *Document) inferLuaDocOperator(receiver, operand TypeSet, metamethod string) (TypeSet, bool) {
	if doc.Server == nil || receiver.CustomName == "" {
		return TypeSet{}, false
	}
	operation := strings.TrimPrefix(metamethod, "__")
	seen := make(map[string]struct{})
	queue := []string{receiver.CustomName}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		entry := doc.Server.firstVisibleLuaDocEntry(doc, ast.HashBytes([]byte(name)))
		if entry == nil {
			continue
		}
		if entry.LuaDoc != nil {
			for _, operator := range entry.LuaDoc.Operators {
				if operator.Operation != operation {
					continue
				}
				if operator.InputType != "" {
					input := ParseTypeString(operator.InputType)
					if input.CustomName != "" {
						if !doc.Server.luaDocClassAssignable(doc, operand.CustomName, input.CustomName) {
							continue
						}
					}
					if input.CustomName == "" && input.Basics&TypeAny == 0 && input.Basics&operand.Basics == 0 {
						continue
					}
				}
				return ParseTypeString(operator.ResultType), true
			}
		}
		queue = append(queue, symbolParents(entry)...)
	}
	return TypeSet{}, false
}

func (doc *Document) inferLuaDocCallOverload(receiver TypeSet, args []TypeSet) (TypeSet, bool) {
	if doc.Server == nil || receiver.CustomName == "" {
		return TypeSet{}, false
	}
	seen := make(map[string]struct{})
	queue := []string{receiver.CustomName}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		entry := doc.Server.inferFirstGlobalSymbolEntry(doc, 0, ast.HashBytes([]byte(name)))
		if entry == nil {
			continue
		}
		if entry.LuaDoc != nil {
			for _, overload := range entry.LuaDoc.Overloads {
				callable := ParseTypeString(overload)
				if len(callable.CallRet) == 0 || len(args) > len(callable.CallParams) {
					continue
				}
				matches := true
				for _, param := range callable.CallParams[len(args):] {
					if !strings.HasSuffix(strings.TrimSpace(param.Type), "?") {
						matches = false
						break
					}
				}
				if !matches {
					continue
				}
				for i, param := range callable.CallParams[:len(args)] {
					expected := ParseTypeString(param.Type)
					if expected.Basics&TypeAny != 0 || expected.Basics&args[i].Basics != 0 {
						continue
					}
					if expected.CustomName != "" {
						if !doc.Server.luaDocClassAssignable(doc, args[i].CustomName, expected.CustomName) {
							matches = false
							break
						}
					} else if expected.Basics&TypeAny == 0 && expected.Basics&args[i].Basics == 0 {
						matches = false
						break
					}
				}
				if matches {
					return ParseTypeString(callable.CallRet[0].Type), true
				}
			}
		}
		queue = append(queue, symbolParents(entry)...)
	}
	return TypeSet{}, false
}

func (s *Server) luaDocClassAssignable(srcDoc *Document, actual, expected string) bool {
	if actual == "" || expected == "" {
		return false
	}
	queue := []string{actual}
	visited := make(map[string]struct{})
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if name == expected {
			return true
		}
		if name == "" {
			continue
		}
		if _, seen := visited[name]; seen {
			continue
		}
		visited[name] = struct{}{}
		entry := s.firstVisibleLuaDocEntry(srcDoc, ast.HashBytes([]byte(name)))
		queue = append(queue, symbolParents(entry)...)
	}
	return false
}

// firstVisibleLuaDocEntry selects the same stable priority as global symbol
// lookup without constructing and sorting a temporary GlobalSymbol slice.
func (s *Server) firstVisibleLuaDocEntry(srcDoc *Document, hash uint64) *SymbolEntry {
	if s == nil || s.GlobalIndex == nil {
		return nil
	}
	var best *SymbolEntry
	bestPriority := 0
	for _, entry := range s.GlobalIndex.SymbolsByHash(GlobalKey{PropHash: hash}) {
		if entry == nil {
			continue
		}
		target := s.Documents[entry.URI]
		if target == nil || !s.canSeeSymbol(srcDoc, target) {
			continue
		}
		priority := s.globalSymbolPriority(entry.URI)
		if best == nil || priority < bestPriority {
			best, bestPriority = entry, priority
		}
	}
	return best
}
