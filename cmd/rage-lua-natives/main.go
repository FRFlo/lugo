package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	outputDir := flag.String("out", filepath.Join("lsp", "stdlib"), "output directory")
	games := flag.String("games", "", "comma-separated games: gta,rdr3,cfx")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cleanupNativeFiles(*outputDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	selected := parseGameTypes(*games)
	for _, game := range selected {
		if err := generateGame(*outputDir, game); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func generateGame(outputDir string, game GameType) error {
	resp, err := http.Get(game.JSONURL())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: %s", game.JSONURL(), resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var raw map[string]map[string]NativeDefinition
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	writer := newLuaWriter(outputDir, game.DocsURL())
	return writer.write(game, raw)
}

type luaWriter struct {
	outputDir string
	docsURL   string
}

func newLuaWriter(outputDir, docsURL string) *luaWriter {
	return &luaWriter{outputDir: outputDir, docsURL: docsURL}
}

func (w *luaWriter) write(game GameType, raw map[string]map[string]NativeDefinition) error {
	grouped := make(map[string][]nativeRecord)
	for namespace, natives := range raw {
		for hash, native := range natives {
			apiSet := normalizeApiSet(native.Apiset)
			grouped[apiSet] = append(grouped[apiSet], nativeRecord{
				Hash:      hash,
				Namespace: namespace,
				Native:    native,
			})
		}
	}

	for _, apiSet := range []string{"client", "server", "shared"} {
		if err := w.writeApiSet(game.String(), apiSet, grouped[apiSet]); err != nil {
			return err
		}
	}

	return nil
}

type nativeRecord struct {
	Hash      string
	Namespace string
	Native    NativeDefinition
}

func normalizeApiSet(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "server", "client", "shared":
		return value
	case "":
		return "client"
	default:
		return "client"
	}
}

func cleanupNativeFiles(outputDir string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}

	keep := map[string]struct{}{
		"natives_gtav_client.lua": {},
		"natives_gtav_server.lua": {},
		"natives_gtav_shared.lua": {},
		"natives_rdr3_client.lua": {},
		"natives_rdr3_server.lua": {},
		"natives_rdr3_shared.lua": {},
		"natives_cfx_client.lua":  {},
		"natives_cfx_server.lua":  {},
		"natives_cfx_shared.lua":  {},
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "natives_") || !strings.HasSuffix(name, ".lua") {
			continue
		}
		if _, ok := keep[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(outputDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

func (w *luaWriter) writeApiSet(game, apiSet string, natives []nativeRecord) error {
	return w.writeApiSetWithGame(game, apiSet, natives)
}

func (w *luaWriter) writeApiSetWithGame(game, apiSet string, natives []nativeRecord) error {
	fileName := "natives_" + apiSet + ".lua"
	if game != "" {
		fileName = "natives_" + game + "_" + apiSet + ".lua"
	}

	if len(natives) == 0 {
		path := filepath.Join(w.outputDir, fileName)
		return os.WriteFile(path, []byte("---@meta\n"), 0o644)
	}

	sort.SliceStable(natives, func(i, j int) bool {
		if natives[i].Namespace == natives[j].Namespace {
			return natives[i].Hash < natives[j].Hash
		}
		return natives[i].Namespace < natives[j].Namespace
	})

	var buf bytes.Buffer
	buf.WriteString("---@meta\n\n")
	for _, item := range natives {
		fnName := nativeName(item.Native, item.Hash)
		buf.WriteString(nativeDescription(item.Native.Description, item.Hash, item.Namespace, item.Native.Apiset, w.docsURL))
		buf.WriteByte('\n')

		params, docParams := nativeParams(item.Native)
		buf.WriteString(docParams)

		retTypes, outParams := convertOutParams(item.Native)
		_ = outParams
		for _, rt := range retTypes {
			if rt == "void" || rt == "" {
				continue
			}
			buf.WriteString("---@return ")
			buf.WriteString(getNativeType(rt, false))
			buf.WriteByte('\n')
		}

		buf.WriteString("function ")
		buf.WriteString(fnName)
		buf.WriteByte('(')
		buf.WriteString(params)
		buf.WriteString(") end\n\n")

		for _, alias := range getAliases(fnName, item.Native.Aliases, item.Native.OldNames) {
			buf.WriteString(alias)
		}
	}

	return os.WriteFile(filepath.Join(w.outputDir, fileName), buf.Bytes(), 0o644)
}

func nativeDescription(description, hash, namespace, apiset, docsURL string) string {
	base := description
	if strings.TrimSpace(base) == "" {
		base = "This native does not have an official description."
	}
	lines := strings.Split(base, "\n")
	for i, line := range lines {
		lines[i] = "---" + line
	}
	return fmt.Sprintf("---**`%s` `%s`**  \n---[Native Documentation](%s%s)  \n%s", namespace, defaultString(apiset, "client"), docsURL, hash, strings.Join(lines, "\n"))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func nativeParams(data NativeDefinition) (string, string) {
	params := make([]string, 0, len(data.Params))
	docParams := make([]string, 0, len(data.Params))
	for _, p := range data.Params {
		if strings.Contains(p.Type, "*") {
			continue
		}
		params = append(params, fieldToReplace(p.Name))
		docParams = append(docParams, "---@param "+fieldToReplace(p.Name)+" "+getNativeType(p.Type, true))
	}
	if len(docParams) > 0 {
		return strings.Join(params, ", "), strings.Join(docParams, "\n") + "\n"
	}
	return strings.Join(params, ", "), ""
}

func convertOutParams(data NativeDefinition) ([]string, []NativeParam) {
	params := append([]NativeParam(nil), data.Params...)
	returnType := data.Results
	if returnType == "" {
		returnType = data.ReturnType
	}
	if returnType == "" {
		returnType = "void"
	}
	newReturnTypes := []string{returnType}
	for i := range params {
		typ := strings.ToLower(strings.ReplaceAll(params[i].Type, "Object", "object_1"))
		params[i].Type = typ
		if !strings.Contains(typ, "*") {
			continue
		}
		if returnType == "void" && len(newReturnTypes) == 1 && newReturnTypes[0] == "void" {
			newReturnTypes = newReturnTypes[:0]
		}
		typ = strings.TrimSuffix(typ, "*")
		typ = strings.TrimPrefix(typ, "const ")
		if strings.HasSuffix(typ, "char") {
			params[i].Type = typ
			continue
		}
		newReturnTypes = append(newReturnTypes, typ)
	}
	return newReturnTypes, params
}

func getAliases(nativeName string, aliasGroups ...[]string) []string {
	var all []string
	for _, group := range aliasGroups {
		all = append(all, group...)
	}
	if len(all) == 0 {
		return nil
	}
	lines := make([]string, 0, len(all))
	for _, alias := range all {
		if strings.HasPrefix(alias, "0") {
			continue
		}
		alias = strings.ToLower(alias)
		alias = strings.ReplaceAll(alias, "0x", "n_0x")
		alias = toLuaIdent(alias)
		if alias == nativeName {
			continue
		}
		lines = append(lines, "---@deprecated\n"+alias+" = "+nativeName+"\n")
	}
	return lines
}

func toLuaIdent(value string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range value {
		switch {
		case r == '_' || r == '-' || r == ' ':
			upperNext = true
		case upperNext:
			b.WriteString(strings.ToUpper(string(r)))
			upperNext = false
		default:
			b.WriteString(strings.ToLower(string(r)))
		}
	}
	return b.String()
}
