package lsp

import (
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMNativeMetadata identifies the catalog metadata attached to an embedded native.
// Native profile selection remains owned by the resolver; this only formats metadata
// from the already selected virtual stdlib document.
type fiveMNativeMetadata struct {
	Namespace string
	Context   string
	Doc       *LuaDoc
}

func fiveMNativeMetadataFor(doc *Document, defID ast.NodeID) (fiveMNativeMetadata, bool) {
	if doc == nil || !strings.HasPrefix(doc.URI, embeddedStdlibURIPrefix) || !strings.Contains(doc.URI, "natives_") {
		return fiveMNativeMetadata{}, false
	}
	if defID == ast.InvalidNode || int(defID) >= len(doc.Tree.Nodes) {
		return fiveMNativeMetadata{}, false
	}

	luadoc := doc.GetLuaDoc(defID)
	if luadoc == nil {
		return fiveMNativeMetadata{}, false
	}

	// Generated natives begin with: **`NAMESPACE` `client|server|shared`**.
	line := strings.TrimSpace(strings.SplitN(luadoc.Description, "\n", 2)[0])
	line = strings.Trim(line, "*")
	rawParts := strings.FieldsFunc(line, func(r rune) bool { return r == '`' || r == '*' })
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) < 2 {
		return fiveMNativeMetadata{}, false
	}

	return fiveMNativeMetadata{Namespace: parts[0], Context: parts[1], Doc: luadoc}, true
}

func fiveMNativeDetail(meta fiveMNativeMetadata) string {
	return "native " + meta.Namespace + " (" + meta.Context + ")"
}

func fiveMNativeDocumentation(meta fiveMNativeMetadata) string {
	if meta.Doc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("**Native** `" + meta.Namespace + "` · **" + meta.Context + "**")
	if meta.Doc.IsDeprecated {
		b.WriteString("\n\n**@deprecated**")
		if meta.Doc.DeprecatedMsg != "" {
			b.WriteString(" - " + meta.Doc.DeprecatedMsg)
		}
	}
	if meta.Doc.Description != "" {
		lines := strings.Split(meta.Doc.Description, "\n")
		// The generated header and documentation link are metadata, not the note.
		if len(lines) > 0 && strings.Contains(lines[0], "**") {
			lines = lines[1:]
		}
		for len(lines) > 0 && (strings.TrimSpace(lines[0]) == "" || strings.Contains(lines[0], "Native Documentation")) {
			lines = lines[1:]
		}
		if len(lines) > 0 {
			b.WriteString("\n\n" + strings.Join(lines, "\n"))
		}
	}
	return b.String()
}
