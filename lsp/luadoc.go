package lsp

import (
	"bytes"
	"strings"
)

var (
	tagParam      = []byte("@param")
	tagReturn     = []byte("@return")
	tagField      = []byte("@field")
	tagDeprecated = []byte("@deprecated")
	tagClass      = []byte("@class")
	tagType       = []byte("@type")
	tagAlias      = []byte("@alias")
	tagGeneric    = []byte("@generic")
	tagOverload   = []byte("@overload")
	tagSee        = []byte("@see")
	tagExport     = []byte("@export")
	tagEnum       = []byte("@enum")
	tagModule     = []byte("@module")
	tagNoDiscard  = []byte("@nodiscard")
	tagAsync      = []byte("@async")
	tagPackage    = []byte("@package")
	tagOperator   = []byte("@operator")

	kwPublic    = []byte("public ")
	kwPrivate   = []byte("private ")
	kwProtected = []byte("protected ")

	dashPrefix = []byte("- ")
	hashPrefix = []byte("# ")
	qSuffix    = []byte("?")
)

type LuaDocParam struct {
	Name string
	Type string
	Desc string
}

type LuaDocReturn struct {
	Type string
	Desc string
}

type LuaDocField struct {
	Name string
	Type string
	Desc string
}

type LuaDocClass struct {
	Name          string
	Parent        string
	Parents       []string
	GenericParams []LuaDocGeneric
	Desc          string
}

type LuaDocType struct {
	Type string
	Desc string
}

type LuaDocAlias struct {
	Name string
	Type string
	Desc string
}

type LuaDocEnum struct {
	Name string
	Desc string
}

type LuaDocModule struct {
	Name string
	Desc string
}

type LuaDocGeneric struct {
	Name   string
	Parent string
}

// LuaDocOperator describes the operand (when present) and result types for a
// Lua metamethod operator annotation.
type LuaDocOperator struct {
	Operation  string
	InputType  string
	ResultType string
}

type LuaDoc struct {
	Description   string
	DeprecatedMsg string
	Class         *LuaDocClass
	Type          *LuaDocType
	Alias         *LuaDocAlias
	Enum          *LuaDocEnum
	Module        *LuaDocModule
	Export        string
	Generics      []LuaDocGeneric
	Params        []LuaDocParam
	Returns       []LuaDocReturn
	Fields        []LuaDocField
	Overloads     []string
	Operators     []LuaDocOperator
	See           []string
	IsDeprecated  bool
	IsNoDiscard   bool
	IsAsync       bool
	IsPackage     bool
}

var luaDocOperators = map[string]struct{}{
	"add": {}, "sub": {}, "mul": {}, "div": {}, "mod": {}, "pow": {}, "unm": {},
	"concat": {}, "len": {}, "eq": {}, "lt": {}, "le": {}, "band": {}, "bor": {},
	"bxor": {}, "bnot": {}, "shl": {}, "shr": {}, "idiv": {}, "call": {}, "index": {},
}

func parseLuaDocOperator(data []byte) (LuaDocOperator, bool) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return LuaDocOperator{}, false
	}

	opEnd := 0
	for opEnd < len(data) && data[opEnd] != '(' && data[opEnd] != ':' && data[opEnd] != ' ' && data[opEnd] != '\t' {
		opEnd++
	}
	operation := string(data[:opEnd])
	if _, ok := luaDocOperators[operation]; !ok {
		return LuaDocOperator{}, false
	}
	data = bytes.TrimSpace(data[opEnd:])
	var input []byte
	if len(data) > 0 && data[0] == '(' {
		depth, end := 0, -1
		for i, c := range data {
			if c == '(' {
				depth++
			}
			if c == ')' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		if end < 0 {
			return LuaDocOperator{}, false
		}
		input = bytes.TrimSpace(data[1:end])
		if len(input) == 0 {
			return LuaDocOperator{}, false
		}
		data = bytes.TrimSpace(data[end+1:])
	}
	if len(data) == 0 || data[0] != ':' {
		return LuaDocOperator{}, false
	}
	result := bytes.TrimSpace(data[1:])
	if len(result) == 0 {
		return LuaDocOperator{}, false
	}
	return LuaDocOperator{Operation: operation, InputType: string(input), ResultType: string(result)}, true
}

// findTypeEnd safely scans past complex types with spaces like 'fun(a: string): number'
func findTypeEnd(data []byte) int {
	var depth int

	for i := 0; i < len(data); i++ {
		char := data[i]

		if char == '(' || char == '<' || char == '{' || char == '[' {
			depth++
		} else if char == ')' || char == '>' || char == '}' || char == ']' {
			depth--
		} else if (char == ' ' || char == '\t') && depth <= 0 {
			var j int

			for j = i - 1; j >= 0 && (data[j] == ' ' || data[j] == '\t'); j-- {
			}

			if j >= 0 && (data[j] == ':' || data[j] == ',' || data[j] == '|') {
				continue // Type is still going (e.g., 'fun(): boolean' or 'string | number')
			}

			var k int

			for k = i + 1; k < len(data) && (data[k] == ' ' || data[k] == '\t'); k++ {
			}

			if k < len(data) && (data[k] == '|' || data[k] == ',') {
				i = k - 1

				continue
			}

			return i
		}
	}

	return -1
}

func extractTypeDesc(data []byte) (typeBytes, descBytes []byte) {
	data = bytes.TrimSpace(data)

	idx := findTypeEnd(data)
	if idx == -1 {
		return data, nil
	}

	return data[:idx], bytes.TrimSpace(data[idx:])
}

type callableType struct {
	Signature string
	Params    []LuaDocParam
	Returns   []LuaDocReturn
}

func splitTopLevelList(value string, sep byte) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	var (
		parts []string
		depth int
		start int
	)

	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(', '<', '{', '[':
			depth++
		case ')', '>', '}', ']':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:i]))
				start = i + 1
			}
		}
	}

	parts = append(parts, strings.TrimSpace(value[start:]))

	return parts
}

func indexTopLevelByte(value string, target byte) int {
	depth := 0

	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(', '<', '{', '[':
			depth++
		case ')', '>', '}', ']':
			depth--
		case target:
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

func parseCallableType(typeStr string) (callableType, bool) {
	trimmed := strings.TrimSpace(typeStr)
	regular := strings.HasPrefix(trimmed, "fun(")
	proxy := strings.HasPrefix(trimmed, "funcref(")
	if !regular && !proxy {
		return callableType{}, false
	}

	openIdx := strings.IndexByte(trimmed, '(')
	if openIdx == -1 {
		return callableType{}, false
	}

	depth := 0
	closeIdx := -1
	for i := openIdx; i < len(trimmed); i++ {
		switch trimmed[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}

	if closeIdx == -1 {
		return callableType{}, false
	}

	callable := callableType{}
	paramsStr := strings.TrimSpace(trimmed[openIdx+1 : closeIdx])
	for _, rawParam := range splitTopLevelList(paramsStr, ',') {
		if rawParam == "" {
			continue
		}

		param := LuaDocParam{}
		colonIdx := indexTopLevelByte(rawParam, ':')
		if colonIdx == -1 {
			param.Name = strings.TrimSpace(rawParam)
		} else {
			param.Name = strings.TrimSpace(rawParam[:colonIdx])
			param.Type = strings.TrimSpace(rawParam[colonIdx+1:])
		}

		if strings.HasSuffix(param.Name, "?") {
			param.Name = strings.TrimSuffix(param.Name, "?")
			if param.Type != "" && !strings.HasSuffix(param.Type, "?") {
				param.Type += "?"
			}
		}

		callable.Params = append(callable.Params, param)
	}

	remainder := strings.TrimSpace(trimmed[closeIdx+1:])
	if after, ok := strings.CutPrefix(remainder, ":"); ok {
		retType := strings.TrimSpace(after)
		if retType != "" {
			callable.Returns = append(callable.Returns, LuaDocReturn{Type: retType})
		}
	}

	callable.Signature = buildCallableSignature(callable.Params, callable.Returns)
	if proxy && callable.Signature == "" {
		callable.Signature = "fun()"
	}

	return callable, true
}

func buildCallableSignature(params []LuaDocParam, returns []LuaDocReturn) string {
	var b strings.Builder

	b.WriteString("fun(")
	for i, param := range params {
		if i > 0 {
			b.WriteString(", ")
		}

		if param.Name != "" {
			b.WriteString(param.Name)
		}

		if param.Type != "" {
			if param.Name != "" {
				b.WriteString(": ")
			}
			b.WriteString(param.Type)
		}
	}
	b.WriteByte(')')

	if len(returns) == 1 && returns[0].Type != "" {
		b.WriteString(": ")
		b.WriteString(returns[0].Type)
	}

	return b.String()
}

func formatAlerts(text string) string {
	if text == "" {
		return ""
	}

	var hasAlert bool

	prefixes := []string{"NOTE:", "TODO:", "INFO:", "WARNING:", "WARN:", "IMPORTANT:", "FIXME:", "BUG:", "TIP:", "CAUTION:"}

	for _, p := range prefixes {
		if strings.Contains(text, p) {
			hasAlert = true

			break
		}
	}

	if !hasAlert {
		return text
	}

	lines := strings.Split(text, "\n")

	var sb strings.Builder

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if len(trimmed) > 0 {
			leadingSpaces := line[:len(line)-len(trimmed)]

			if strings.HasPrefix(trimmed, "NOTE:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("ℹ️ **NOTE:**")
				sb.WriteString(trimmed[5:])
			} else if strings.HasPrefix(trimmed, "TODO:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("📝 **TODO:**")
				sb.WriteString(trimmed[5:])
			} else if strings.HasPrefix(trimmed, "INFO:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("ℹ️ **INFO:**")
				sb.WriteString(trimmed[5:])
			} else if strings.HasPrefix(trimmed, "WARNING:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("⚠️ **WARNING:**")
				sb.WriteString(trimmed[8:])
			} else if strings.HasPrefix(trimmed, "WARN:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("⚠️ **WARN:**")
				sb.WriteString(trimmed[5:])
			} else if strings.HasPrefix(trimmed, "IMPORTANT:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("❗ **IMPORTANT:**")
				sb.WriteString(trimmed[10:])
			} else if strings.HasPrefix(trimmed, "FIXME:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("🔧 **FIXME:**")
				sb.WriteString(trimmed[6:])
			} else if strings.HasPrefix(trimmed, "BUG:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("🐛 **BUG:**")
				sb.WriteString(trimmed[4:])
			} else if strings.HasPrefix(trimmed, "TIP:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("💡 **TIP:**")
				sb.WriteString(trimmed[4:])
			} else if strings.HasPrefix(trimmed, "CAUTION:") {
				sb.WriteString(leadingSpaces)
				sb.WriteString("🛑 **CAUTION:**")
				sb.WriteString(trimmed[8:])
			} else {
				sb.WriteString(line)
			}
		} else {
			sb.WriteString(line)
		}

		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

func extractNameDesc(data []byte) (name, descBytes []byte) {
	data = bytes.TrimSpace(data)

	nameEnd := 0
	for nameEnd < len(data) && data[nameEnd] != ' ' && data[nameEnd] != '\t' {
		nameEnd++
	}
	if nameEnd == 0 {
		return nil, data
	}

	return data[:nameEnd], bytes.TrimSpace(data[nameEnd:])
}

func extractNameParent(data []byte) (name, parent, descBytes []byte) {
	data = bytes.TrimSpace(data)

	var nameEnd int

	for nameEnd < len(data) && data[nameEnd] != ' ' && data[nameEnd] != '\t' && data[nameEnd] != ':' {
		nameEnd++
	}

	if nameEnd == 0 {
		return nil, nil, data
	}

	name = data[:nameEnd]
	data = bytes.TrimSpace(data[nameEnd:])

	if bytes.HasPrefix(data, []byte(":")) {
		data = bytes.TrimSpace(data[1:])

		var parentEnd int

		for parentEnd < len(data) && data[parentEnd] != ' ' && data[parentEnd] != '\t' {
			parentEnd++
		}

		parent = data[:parentEnd]
		descBytes = bytes.TrimSpace(data[parentEnd:])
	} else {
		descBytes = data
	}

	return name, parent, descBytes
}

func extractNameParents(data []byte) (name []byte, parents [][]byte, descBytes []byte) {
	data = bytes.TrimSpace(data)

	nameEnd := luaDocClassHeaderEnd(data)
	if nameEnd == 0 {
		return nil, nil, data
	}
	name = data[:nameEnd]
	// LuaCATS declares generic classes as `Class<T>`, while references to the
	// class itself are indexed by its base name (`Class`).
	if genericStart := bytes.IndexByte(name, '<'); genericStart > 0 && bytes.HasSuffix(name, []byte(">")) {
		name = name[:genericStart]
	}
	data = bytes.TrimSpace(data[nameEnd:])
	if !bytes.HasPrefix(data, []byte(":")) {
		return name, nil, data
	}
	data = bytes.TrimSpace(data[1:])
	for len(data) > 0 {
		end := 0
		angleDepth := 0
		parenDepth := 0
		for end < len(data) {
			switch data[end] {
			case '<':
				angleDepth++
			case '>':
				if angleDepth > 0 {
					angleDepth--
				}
			case '(':
				parenDepth++
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
			case ',', ' ', '\t':
				if angleDepth == 0 && parenDepth == 0 {
					goto parentEnd
				}
			}
			end++
		}
	parentEnd:
		if end == 0 {
			break
		}
		parents = append(parents, data[:end])
		data = bytes.TrimSpace(data[end:])
		if !bytes.HasPrefix(data, []byte(",")) {
			break
		}
		data = bytes.TrimSpace(data[1:])
	}
	return name, parents, bytes.TrimSpace(data)
}

func luaDocClassHeaderEnd(data []byte) int {
	angleDepth := 0
	for i, char := range data {
		switch char {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ':':
			if angleDepth == 0 {
				return i
			}
		case ' ', '\t':
			if angleDepth == 0 {
				return i
			}
		}
	}
	return len(data)
}

// cutLuaDocTag matches a tag only when its name ends at whitespace or the end
// of the line. Without the boundary, tags such as @parameter are parsed as
// @param.
func cutLuaDocTag(line, tag []byte) ([]byte, bool) {
	after, ok := bytes.CutPrefix(line, tag)
	if !ok || len(after) > 0 && after[0] != ' ' && after[0] != '\t' {
		return nil, false
	}
	return after, true
}

func parseLuaDoc(comments []byte, enableAlerts bool) LuaDoc {
	var (
		doc       LuaDoc
		descLines [][]byte
		activeTag string
	)

	for len(comments) > 0 {
		var line []byte

		idx := bytes.IndexByte(comments, '\n')
		if idx == -1 {
			line = comments
			comments = nil
		} else {
			line = comments[:idx]
			comments = comments[idx+1:]
		}

		line = bytes.TrimSpace(line)

		if after, ok := cutLuaDocTag(line, tagParam); ok {
			activeTag = "param"

			rest := bytes.TrimSpace(after)
			spaceIdx := bytes.IndexByte(rest, ' ')

			if spaceIdx != -1 {
				name := rest[:spaceIdx]
				rest = bytes.TrimSpace(rest[spaceIdx:])

				typeBytes, descBytes := extractTypeDesc(rest)
				descBytes = bytes.TrimPrefix(descBytes, dashPrefix)

				nameStr := string(name)
				typStr := string(typeBytes)

				if bytes.HasSuffix(name, qSuffix) {
					nameStr = nameStr[:len(nameStr)-1]
					typStr += "?"
				}

				doc.Params = append(doc.Params, LuaDocParam{Name: nameStr, Type: typStr, Desc: string(descBytes)})
			} else {
				nameStr := string(rest)

				if bytes.HasSuffix(rest, qSuffix) {
					nameStr = nameStr[:len(nameStr)-1]
				}

				doc.Params = append(doc.Params, LuaDocParam{Name: nameStr})
			}
		} else if after, ok := cutLuaDocTag(line, tagReturn); ok {
			activeTag = "return"

			typeBytes, descBytes := extractTypeDesc(bytes.TrimSpace(after))

			descBytes = bytes.TrimPrefix(descBytes, dashPrefix)
			descBytes = bytes.TrimPrefix(descBytes, hashPrefix)

			doc.Returns = append(doc.Returns, LuaDocReturn{Type: string(typeBytes), Desc: string(descBytes)})
		} else if after, ok := cutLuaDocTag(line, tagField); ok {
			activeTag = "field"

			rest := bytes.TrimSpace(after)

			if bytes.HasPrefix(rest, kwPublic) {
				rest = bytes.TrimSpace(rest[7:])
			} else if bytes.HasPrefix(rest, kwPrivate) {
				rest = bytes.TrimSpace(rest[8:])
			} else if bytes.HasPrefix(rest, kwProtected) {
				rest = bytes.TrimSpace(rest[10:])
			}

			spaceIdx := bytes.IndexByte(rest, ' ')
			if spaceIdx != -1 {
				name := rest[:spaceIdx]
				rest = bytes.TrimSpace(rest[spaceIdx:])

				typeBytes, descBytes := extractTypeDesc(rest)
				descBytes = bytes.TrimPrefix(descBytes, dashPrefix)

				nameStr := string(name)
				typStr := string(typeBytes)

				if bytes.HasSuffix(name, qSuffix) {
					nameStr = nameStr[:len(nameStr)-1]
					typStr = "?" + typStr
				}

				doc.Fields = append(doc.Fields, LuaDocField{Name: nameStr, Type: typStr, Desc: string(descBytes)})
			} else {
				nameStr := string(rest)

				if bytes.HasSuffix(rest, qSuffix) {
					nameStr = nameStr[:len(nameStr)-1]
				}

				doc.Fields = append(doc.Fields, LuaDocField{Name: nameStr})
			}
		} else if after, ok := cutLuaDocTag(line, tagDeprecated); ok {
			activeTag = ""

			doc.IsDeprecated = true
			doc.DeprecatedMsg = string(bytes.TrimSpace(after))
		} else if after, ok := cutLuaDocTag(line, tagClass); ok {
			activeTag = ""

			classHeader := bytes.TrimSpace(after)
			headerEnd := luaDocClassHeaderEnd(classHeader)
			genericParams := parseLuaDocGenericParams(classHeader[:headerEnd])
			name, parentNames, desc := extractNameParents(after)
			if len(name) > 0 {
				parents := make([]string, 0, len(parentNames))
				for _, parent := range parentNames {
					parents = append(parents, string(parent))
				}
				var parent string
				if len(parents) > 0 {
					parent = parents[0]
				}
				doc.Class = &LuaDocClass{Name: string(name), Parent: parent, Parents: parents, GenericParams: genericParams, Desc: string(desc)}
			}
		} else if after, ok := cutLuaDocTag(line, tagEnum); ok {
			activeTag = ""

			name, desc := extractNameDesc(after)
			if len(name) > 0 {
				doc.Enum = &LuaDocEnum{Name: string(name), Desc: string(desc)}
			}
		} else if after, ok := cutLuaDocTag(line, tagModule); ok {
			activeTag = ""

			name, desc := extractNameDesc(after)
			if len(name) > 0 {
				doc.Module = &LuaDocModule{Name: string(name), Desc: string(desc)}
			}
		} else if _, ok := cutLuaDocTag(line, tagNoDiscard); ok {
			activeTag = ""
			doc.IsNoDiscard = true
		} else if _, ok := cutLuaDocTag(line, tagAsync); ok {
			activeTag = ""
			doc.IsAsync = true
		} else if _, ok := cutLuaDocTag(line, tagPackage); ok {
			activeTag = ""
			doc.IsPackage = true
		} else if after, ok := cutLuaDocTag(line, tagType); ok {
			activeTag = ""

			typeBytes, descBytes := extractTypeDesc(bytes.TrimSpace(after))

			doc.Type = &LuaDocType{Type: string(typeBytes), Desc: string(descBytes)}
		} else if after, ok := cutLuaDocTag(line, tagAlias); ok {
			activeTag = ""

			rest := bytes.TrimSpace(after)

			spaceIdx := bytes.IndexByte(rest, ' ')
			if spaceIdx != -1 {
				name := rest[:spaceIdx]
				rest = bytes.TrimSpace(rest[spaceIdx:])

				typeBytes, descBytes := extractTypeDesc(rest)

				doc.Alias = &LuaDocAlias{Name: string(name), Type: string(typeBytes), Desc: string(descBytes)}
			} else {
				doc.Alias = &LuaDocAlias{Name: string(rest)}
			}
		} else if after, ok := cutLuaDocTag(line, tagGeneric); ok {
			activeTag = ""

			rest := bytes.TrimSpace(after)

			for gStr := range bytes.SplitSeq(rest, []byte(",")) {
				name, parent, _ := extractNameParent(gStr)
				if len(name) > 0 {
					doc.Generics = append(doc.Generics, LuaDocGeneric{Name: string(name), Parent: string(parent)})
				}
			}
		} else if after, ok := cutLuaDocTag(line, tagOverload); ok {
			activeTag = ""

			doc.Overloads = append(doc.Overloads, string(bytes.TrimSpace(after)))
		} else if after, ok := cutLuaDocTag(line, tagOperator); ok {
			activeTag = ""

			if operator, ok := parseLuaDocOperator(after); ok {
				doc.Operators = append(doc.Operators, operator)
			}
		} else if after, ok := cutLuaDocTag(line, tagSee); ok {
			activeTag = ""

			doc.See = append(doc.See, string(bytes.TrimSpace(after)))
		} else if after, ok := cutLuaDocTag(line, tagExport); ok {
			activeTag = ""

			doc.Export = string(bytes.TrimSpace(after))
		} else {
			if len(line) > 0 {
				if line[0] != '@' { // Ignore @meta, @diagnostic, etc.
					switch activeTag {
					case "param":
						if doc.Params[len(doc.Params)-1].Desc != "" {
							doc.Params[len(doc.Params)-1].Desc += "\n" + string(line)
						} else {
							doc.Params[len(doc.Params)-1].Desc = string(line)
						}
					case "return":
						if doc.Returns[len(doc.Returns)-1].Desc != "" {
							doc.Returns[len(doc.Returns)-1].Desc += "\n" + string(line)
						} else {
							doc.Returns[len(doc.Returns)-1].Desc = string(line)
						}
					case "field":
						if doc.Fields[len(doc.Fields)-1].Desc != "" {
							doc.Fields[len(doc.Fields)-1].Desc += "\n" + string(line)
						} else {
							doc.Fields[len(doc.Fields)-1].Desc = string(line)
						}
					default:
						descLines = append(descLines, line)
					}
				} else {
					activeTag = ""

					descLines = append(descLines, line)
				}
			} else {
				activeTag = ""

				descLines = append(descLines, line) // Preserve empty lines for paragraph gaps
			}
		}
	}

	// Clean up leading/trailing empty description lines
	for len(descLines) > 0 && len(descLines[0]) == 0 {
		descLines = descLines[1:]
	}

	for len(descLines) > 0 && len(descLines[len(descLines)-1]) == 0 {
		descLines = descLines[:len(descLines)-1]
	}

	if len(descLines) > 0 {
		doc.Description = string(bytes.Join(descLines, []byte("\n")))
	}

	if enableAlerts {
		doc.Description = formatAlerts(doc.Description)

		for i := range doc.Params {
			doc.Params[i].Desc = formatAlerts(doc.Params[i].Desc)
		}

		for i := range doc.Returns {
			doc.Returns[i].Desc = formatAlerts(doc.Returns[i].Desc)
		}

		for i := range doc.Fields {
			doc.Fields[i].Desc = formatAlerts(doc.Fields[i].Desc)
		}
	}

	return doc
}
