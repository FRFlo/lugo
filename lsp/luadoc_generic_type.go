package lsp

import (
	"strings"
)

// splitLuaDocGenericType extracts the base name and top-level arguments from
// a LuaCATS type such as Map<string, Array<number>>.
func splitLuaDocGenericType(typeName string) (string, []string, bool) {
	typeName = strings.TrimSpace(typeName)
	open := strings.IndexByte(typeName, '<')
	if open <= 0 || !strings.HasSuffix(typeName, ">") {
		return "", nil, false
	}
	depth := 0
	close := -1
	for i := open; i < len(typeName); i++ {
		switch typeName[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				close = i
				if i != len(typeName)-1 {
					return "", nil, false
				}
			}
		}
		if depth < 0 {
			return "", nil, false
		}
	}
	if close != len(typeName)-1 || depth != 0 {
		return "", nil, false
	}
	args := splitLuaDocTypeArguments(typeName[open+1 : close])
	if len(args) == 0 {
		return "", nil, false
	}
	return strings.TrimSpace(typeName[:open]), args, true
}

func splitLuaDocTypeArguments(source string) []string {
	var args []string
	start := 0
	var angle, paren, brace, square int
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '<':
			angle++
		case '>':
			if angle > 0 {
				angle--
			}
		case '(':
			paren++
		case ')':
			if paren > 0 {
				paren--
			}
		case '{':
			brace++
		case '}':
			if brace > 0 {
				brace--
			}
		case '[':
			square++
		case ']':
			if square > 0 {
				square--
			}
		case ',':
			if angle == 0 && paren == 0 && brace == 0 && square == 0 {
				args = append(args, strings.TrimSpace(source[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(source[start:]); tail != "" {
		args = append(args, tail)
	}
	return args
}

func parseLuaDocGenericParams(name []byte) []LuaDocGeneric {
	text := string(name)
	_, parameters, ok := splitLuaDocGenericType(text)
	if !ok {
		return nil
	}
	result := make([]LuaDocGeneric, 0, len(parameters))
	for _, parameter := range parameters {
		genericName, parent, _ := extractNameParent([]byte(parameter))
		if len(genericName) > 0 {
			result = append(result, LuaDocGeneric{Name: string(genericName), Parent: string(parent)})
		}
	}
	return result
}

func luaDocGenericClassName(typeName string) string {
	if name, _, ok := splitLuaDocGenericType(typeName); ok {
		return name
	}
	return strings.TrimSpace(typeName)
}

func luaDocClassHasGenericParent(parents []string) bool {
	for _, parent := range parents {
		if _, _, ok := splitLuaDocGenericType(parent); ok {
			return true
		}
	}
	return false
}
