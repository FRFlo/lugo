//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	fiveMNativeGTACatalogURL = "https://static.cfx.re/natives/natives.json"
	fiveMNativeCFXCatalogURL = "https://static.cfx.re/natives/natives_cfx.json"
	fiveMNativeDocsURL       = "https://docs.fivem.net/natives/?_"
	fiveMNativeGeneratorUA   = "lugo-fivem-native-compile/1.0"
	fiveMNativeHTTPTimeout   = 2 * time.Minute
)

type sourceCatalog map[string]map[string]sourceNative

type sourceNative struct {
	Name        string        `json:"name"`
	Params      []sourceParam `json:"params"`
	Results     string        `json:"results"`
	ReturnType  string        `json:"return_type"`
	Description string        `json:"description"`
	Comment     string        `json:"comment"`
	Hash        string        `json:"hash"`
	APISet      string        `json:"apiset"`
	Game        string        `json:"game"`
	Aliases     []string      `json:"aliases"`
	OldNames    []string      `json:"old_names"`
}

type sourceParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type generatedBundle struct {
	Name        string
	Input       string
	Family      string
	Runtime     string
	Description string
	Natives     []generatedNative
}

type generatedNative struct {
	Name        string
	Description string
	Params      []generatedParam
	Returns     []string
	Aliases     []string
}

type generatedParam struct {
	Name string
	Type string
}

func main() {
	bundles, err := buildFiveMNativeBundles()
	if err != nil {
		fatalf("generate FiveM native bundles: %v", err)
	}

	content, err := renderGeneratedGoSource(bundles)
	if err != nil {
		fatalf("render generated Go source: %v", err)
	}

	if err := os.WriteFile("fivem_native_generated.go", content, 0o644); err != nil {
		fatalf("write fivem_native_generated.go: %v", err)
	}
}

func buildFiveMNativeBundles() ([]generatedBundle, error) {
	client := &http.Client{Timeout: fiveMNativeHTTPTimeout}

	gtaCatalog, err := fetchFiveMNativeCatalog(client, fiveMNativeGTACatalogURL)
	if err != nil {
		return nil, err
	}

	cfxCatalog, err := fetchFiveMNativeCatalog(client, fiveMNativeCFXCatalogURL)
	if err != nil {
		return nil, err
	}

	bundles := []generatedBundle{
		// FiveM still selects these historical GTA client filenames from fxmanifest
		// metadata, but the public native catalogs are live/current instead of
		// per-build snapshots. Keep the filenames for selection and virtual URI
		// compatibility while compiling all GTA client bundles from the same live
		// GTA + compatible CFX metadata.
		buildGTABundle("natives_21e43a33.lua", gtaCatalog, cfxCatalog),
		buildGTABundle("natives_0193d0af.lua", gtaCatalog, cfxCatalog),
		buildGTABundle("natives_universal.lua", gtaCatalog, cfxCatalog),
		buildServerBundle(cfxCatalog),
	}

	for i := range bundles {
		sort.Slice(bundles[i].Natives, func(a, b int) bool { return bundles[i].Natives[a].Name < bundles[i].Natives[b].Name })
	}

	sort.Slice(bundles, func(i, j int) bool { return bundles[i].Name < bundles[j].Name })

	return bundles, nil
}

func fetchFiveMNativeCatalog(client *http.Client, url string) (sourceCatalog, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fiveMNativeGeneratorUA)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}

	var catalog sourceCatalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("decode %s: %w", url, err)
	}

	return catalog, nil
}

func buildGTABundle(name string, gtaCatalog, cfxCatalog sourceCatalog) generatedBundle {
	bundle := generatedBundle{
		Name:        name,
		Input:       "natives.json + natives_cfx.json",
		Family:      "gta5",
		Runtime:     "client",
		Description: "Generated from live FiveM GTA V native metadata plus compatible CFX client/shared natives.",
	}
	bundle.Natives = mergeGeneratedNatives(
		collectGeneratedNatives(gtaCatalog, fiveMNativeDocsURL, func(sourceNative) bool { return true }),
		collectGeneratedNatives(cfxCatalog, fiveMNativeDocsURL, func(n sourceNative) bool {
			if !matchesFiveMGame(n.Game, "gta5") {
				return false
			}
			return allowsFiveMClientAPISet(n.APISet)
		}),
	)
	return bundle
}

func buildServerBundle(cfxCatalog sourceCatalog) generatedBundle {
	bundle := generatedBundle{
		Name:        "natives_server.lua",
		Input:       "natives_cfx.json",
		Family:      "server",
		Runtime:     "server",
		Description: "Generated from live CFX server/shared native metadata.",
	}
	bundle.Natives = collectGeneratedNatives(cfxCatalog, fiveMNativeDocsURL, func(n sourceNative) bool {
		return allowsFiveMServerAPISet(n.APISet)
	})
	return bundle
}

func collectGeneratedNatives(catalog sourceCatalog, docsBase string, include func(sourceNative) bool) []generatedNative {
	result := make([]generatedNative, 0)
	for namespace, natives := range catalog {
		for hash, native := range natives {
			if !include(native) {
				continue
			}
			result = append(result, buildGeneratedNative(namespace, hash, native, docsBase))
		}
	}
	return result
}

func buildGeneratedNative(namespace, hash string, native sourceNative, docsBase string) generatedNative {
	desc := native.Description
	if desc == "" {
		desc = native.Comment
	}

	results := native.Results
	if results == "" {
		results = native.ReturnType
	}

	convertedReturns, convertedParams := convertFiveMOutParams(fiveMNativeNameValue(native, hash), results, native.Params)
	params := make([]generatedParam, 0, len(convertedParams))
	paramNames := make(map[string]int)
	for _, param := range convertedParams {
		paramType := mapFiveMType(param.Type, true)
		baseName := sanitizeFiveMField(param.Name)
		if baseName == "" {
			baseName = "arg"
		}
		name := baseName
		if count := paramNames[baseName]; count > 0 {
			name = fmt.Sprintf("%s%d", baseName, count+1)
		}
		paramNames[baseName]++
		params = append(params, generatedParam{Name: name, Type: paramType})
	}

	returns := make([]string, 0, len(convertedReturns))
	for _, ret := range convertedReturns {
		mapped := mapFiveMType(ret, false)
		if mapped == "void" {
			continue
		}
		returns = append(returns, mapped)
	}

	name := normalizeFiveMNativeName(fiveMNativeNameValue(native, hash))
	aliases := buildFiveMAliases(name, firstNonEmptyStringSlice(native.Aliases, native.OldNames))

	apiSet := native.APISet
	if apiSet == "" {
		apiSet = "client"
	}

	return generatedNative{
		Name:        name,
		Description: buildFiveMDescription(desc, hash, namespace, apiSet, docsBase),
		Params:      params,
		Returns:     returns,
		Aliases:     aliases,
	}
}

func mergeGeneratedNatives(sets ...[]generatedNative) []generatedNative {
	merged := make(map[string]generatedNative)
	for _, set := range sets {
		for _, native := range set {
			existing, ok := merged[native.Name]
			if !ok {
				merged[native.Name] = native
				continue
			}

			if strings.TrimSpace(existing.Description) == "---This native does not have an official description." && strings.TrimSpace(native.Description) != "---This native does not have an official description." {
				existing.Description = native.Description
			}
			if len(existing.Params) == 0 && len(native.Params) > 0 {
				existing.Params = native.Params
			}
			if len(existing.Returns) == 0 && len(native.Returns) > 0 {
				existing.Returns = native.Returns
			}
			existing.Aliases = mergeFiveMAliases(existing.Aliases, native.Aliases)
			merged[native.Name] = existing
		}
	}

	result := make([]generatedNative, 0, len(merged))
	for _, native := range merged {
		result = append(result, native)
	}
	return result
}

func renderGeneratedGoSource(bundles []generatedBundle) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("// Code generated by go generate ./lsp; DO NOT EDIT.\n\n")
	out.WriteString("package lsp\n\n")
	out.WriteString("import (\n\t\"fmt\"\n\t\"strings\"\n)\n\n")
	out.WriteString(`type compiledFiveMNativeBundle struct {
	Name        string
	Input       string
	Family      string
	Runtime     string
	Description string
	Natives     []compiledFiveMNative
}

type compiledFiveMNative struct {
	Name        string
	Description string
	Params      []compiledFiveMNativeParam
	Returns     []string
	Aliases     []string
}

type compiledFiveMNativeParam struct {
	Name string
	Type string
}

`)
	out.WriteString("var compiledFiveMNativeBundles = map[string]compiledFiveMNativeBundle{\n")
	for _, bundle := range bundles {
		renderCompiledBundleLiteral(&out, bundle)
	}
	out.WriteString("}\n\n")
	out.WriteString(`func compiledFiveMNativeCatalogEntries(name string) (map[string]fiveMNativeCatalogEntry, error) {
	if !isFiveMNativeBundleName(name) {
		return nil, fmt.Errorf("unknown FiveM native bundle %q", name)
	}
	bundle, ok := compiledFiveMNativeBundles[name]
	if !ok {
		return nil, fmt.Errorf("missing compiled FiveM native bundle %q; run go generate ./lsp", name)
	}
	entries := make(map[string]fiveMNativeCatalogEntry, len(bundle.Natives))
	for _, native := range bundle.Natives {
		luadoc := compiledFiveMNativeLuaDoc(native)
		paramNames := make([]string, 0, len(native.Params))
		for _, param := range native.Params {
			paramNames = append(paramNames, param.Name)
		}
		entry := fiveMNativeCatalogEntry{
			Name:       native.Name,
			Bundle:     bundle.Name,
			LuaDoc:     luaDocDataFromLuaDoc(luadoc),
			ParamNames: paramNames,
		}
		entry.Type = structuralFunctionTypeFromLuaDoc(entry.ParamNames, luadoc.Params, luadoc.Returns)
		entries[native.Name] = entry
	}
	return entries, nil
}

func compiledFiveMNativeLuaDoc(native compiledFiveMNative) LuaDoc {
	comments := renderCompiledFiveMNativeComments(native)
	cleaned := cleanLuaCommentBytes(nil, []byte(comments))
	return parseLuaDoc(cleaned, false)
}

func readCompiledFiveMNativeBundle(name string) ([]byte, error) {
	if !isFiveMNativeBundleName(name) {
		return nil, fmt.Errorf("unknown FiveM native bundle %q", name)
	}
	bundle, ok := compiledFiveMNativeBundles[name]
	if !ok {
		return nil, fmt.Errorf("missing compiled FiveM native bundle %q; run go generate ./lsp", name)
	}
	return []byte(renderCompiledFiveMNativeBundle(bundle)), nil
}

func renderCompiledFiveMNativeBundle(bundle compiledFiveMNativeBundle) string {
	var sb strings.Builder
	sb.WriteString("---@meta\n\n")
	sb.WriteString(fmt.Sprintf("---Generated from live rage-lua-natives-compatible metadata for %s.\n", bundle.Name))
	sb.WriteString(fmt.Sprintf("---Source input: %s (%s %s).\n", bundle.Input, bundle.Family, bundle.Runtime))
	if bundle.Description != "" {
		sb.WriteString("---")
		sb.WriteString(bundle.Description)
		sb.WriteByte('\n')
	}
	for i, native := range bundle.Natives {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(renderCompiledFiveMNative(native))
	}
	return sb.String()
}

func renderCompiledFiveMNative(native compiledFiveMNative) string {
	var sb strings.Builder
	sb.WriteString(renderCompiledFiveMNativeComments(native))
	sb.WriteString("function ")
	sb.WriteString(native.Name)
	sb.WriteByte('(')
	for i, param := range native.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(param.Name)
	}
	sb.WriteString(") end\n")
	for _, alias := range native.Aliases {
		sb.WriteString("\n---@deprecated\n")
		sb.WriteString(alias)
		sb.WriteString(" = ")
		sb.WriteString(native.Name)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func renderCompiledFiveMNativeComments(native compiledFiveMNative) string {
	var sb strings.Builder
	if native.Description != "" {
		sb.WriteString(native.Description)
		sb.WriteByte('\n')
	}
	for _, param := range native.Params {
		sb.WriteString("---@param ")
		sb.WriteString(param.Name)
		sb.WriteByte(' ')
		sb.WriteString(param.Type)
		sb.WriteByte('\n')
	}
	for _, ret := range native.Returns {
		sb.WriteString("---@return ")
		sb.WriteString(ret)
		sb.WriteByte('\n')
	}
	return sb.String()
}
`)

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

func renderCompiledBundleLiteral(out *bytes.Buffer, bundle generatedBundle) {
	out.WriteString("\t")
	out.WriteString(strconv.Quote(bundle.Name))
	out.WriteString(": {\n")
	out.WriteString("\t\tName: ")
	out.WriteString(strconv.Quote(bundle.Name))
	out.WriteString(",\n")
	out.WriteString("\t\tInput: ")
	out.WriteString(strconv.Quote(bundle.Input))
	out.WriteString(",\n")
	out.WriteString("\t\tFamily: ")
	out.WriteString(strconv.Quote(bundle.Family))
	out.WriteString(",\n")
	out.WriteString("\t\tRuntime: ")
	out.WriteString(strconv.Quote(bundle.Runtime))
	out.WriteString(",\n")
	out.WriteString("\t\tDescription: ")
	out.WriteString(strconv.Quote(bundle.Description))
	out.WriteString(",\n")
	out.WriteString("\t\tNatives: []compiledFiveMNative{\n")
	for _, native := range bundle.Natives {
		renderCompiledNativeLiteral(out, native)
	}
	out.WriteString("\t\t},\n")
	out.WriteString("\t},\n")
}

func renderCompiledNativeLiteral(out *bytes.Buffer, native generatedNative) {
	out.WriteString("\t\t\t{\n")
	out.WriteString("\t\t\t\tName: ")
	out.WriteString(strconv.Quote(native.Name))
	out.WriteString(",\n")
	out.WriteString("\t\t\t\tDescription: ")
	out.WriteString(strconv.Quote(native.Description))
	out.WriteString(",\n")
	out.WriteString("\t\t\t\tParams: []compiledFiveMNativeParam{")
	for _, param := range native.Params {
		out.WriteString("{Name: ")
		out.WriteString(strconv.Quote(param.Name))
		out.WriteString(", Type: ")
		out.WriteString(strconv.Quote(param.Type))
		out.WriteString("},")
	}
	out.WriteString("},\n")
	out.WriteString("\t\t\t\tReturns: ")
	renderStringSliceLiteral(out, native.Returns)
	out.WriteString(",\n")
	out.WriteString("\t\t\t\tAliases: ")
	renderStringSliceLiteral(out, native.Aliases)
	out.WriteString(",\n")
	out.WriteString("\t\t\t},\n")
}

func renderStringSliceLiteral(out *bytes.Buffer, values []string) {
	if len(values) == 0 {
		out.WriteString("nil")
		return
	}
	out.WriteString("[]string{")
	for _, value := range values {
		out.WriteString(strconv.Quote(value))
		out.WriteString(",")
	}
	out.WriteString("}")
}

func fiveMNativeNameValue(native sourceNative, hash string) string {
	if native.Name != "" {
		return native.Name
	}
	if native.Hash != "" {
		return native.Hash
	}
	return hash
}

func normalizeFiveMNativeName(name string) string {
	name = strings.ToLower(name)
	name = strings.Replace(name, "0x", "n_0x", 1)
	var out strings.Builder
	upperNext := false
	for i, r := range name {
		switch {
		case i == 0:
			out.WriteString(strings.ToUpper(string(r)))
		case r == '_':
			upperNext = true
		case upperNext && r >= 'a' && r <= 'z':
			out.WriteRune(r - ('a' - 'A'))
			upperNext = false
		default:
			out.WriteRune(r)
			upperNext = false
		}
	}
	return out.String()
}

func buildFiveMAliases(nativeName string, aliases []string) []string {
	result := make([]string, 0, len(aliases))
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		if alias == "" || strings.HasPrefix(alias, "0") {
			continue
		}
		normalized := normalizeFiveMNativeName(alias)
		if normalized == nativeName {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func convertFiveMOutParams(nativeName, results string, params []sourceParam) ([]string, []sourceParam) {
	returnTypes := []string{separateFiveMObjectTypes(firstNonEmptyString(results, "void"))}
	keptParams := make([]sourceParam, 0, len(params))
	for _, param := range params {
		typeName := strings.ToLower(separateFiveMObjectTypes(param.Type))
		if !strings.Contains(typeName, "*") {
			keptParams = append(keptParams, sourceParam{Name: param.Name, Type: typeName})
			continue
		}

		trimmed := strings.TrimSuffix(typeName, "*")
		trimmed = strings.TrimPrefix(trimmed, "const ")
		trimmed = strings.TrimSpace(trimmed)

		if isFiveMNonReturnPointerNative(nativeName) || trimmed == "char" {
			keptParams = append(keptParams, sourceParam{Name: param.Name, Type: trimmed})
			continue
		}

		if len(returnTypes) == 1 && returnTypes[0] == "void" {
			returnTypes = returnTypes[:0]
		}
		returnTypes = append(returnTypes, trimmed)
	}
	return returnTypes, keptParams
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func mergeFiveMAliases(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))
	for _, alias := range append(append([]string{}, a...), b...) {
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		result = append(result, alias)
	}
	sort.Strings(result)
	return result
}

func buildFiveMDescription(description, hash, namespace, apiset, docsBase string) string {
	baseDesc := description
	if strings.TrimSpace(baseDesc) == "" {
		baseDesc = "This native does not have an official description."
	}
	baseDesc = strings.ReplaceAll(baseDesc, "\r\n", "\n")
	baseDesc = strings.ReplaceAll(baseDesc, "\r", "\n")
	lines := strings.Split(baseDesc, "\n")
	for i, line := range lines {
		lines[i] = "---" + line
	}
	return fmt.Sprintf("---**`%s` `%s`**  \n---[Native Documentation](%s%s)  \n%s", namespace, firstNonEmptyString(apiset, "client"), docsBase, hash, strings.Join(lines, "\n"))
}

func sanitizeFiveMField(field string) string {
	switch field {
	case "end", "repeat", "local":
		return "_" + field
	default:
		return field
	}
}

func mapFiveMType(typeName string, input bool) string {
	typeName = strings.ToLower(strings.TrimSpace(typeName))
	switch typeName {
	case "vector3", "string", "void":
		return typeName
	case "char", "char*":
		return "string"
	case "hash":
		if input {
			return "integer | string"
		}
		return "integer"
	case "bool":
		return "boolean"
	case "object":
		return "table"
	case "func":
		return "function"
	case "float":
		return "number"
	case "uint", "entity", "player", "decisionmaker", "fireid", "ped", "vehicle", "cam", "cargenerator", "group", "train", "pickup", "object_1", "weapon", "interior", "blip", "texture", "texturedict", "coverpoint", "camera", "tasksequence", "sphere", "scrhandle", "int", "long", "itemset", "animscene", "perschar", "popzone", "prompt", "propset", "volume":
		return "integer"
	default:
		return "any"
	}
}

func separateFiveMObjectTypes(typeName string) string {
	if strings.Contains(typeName, "Object") {
		return strings.ReplaceAll(typeName, "Object", "object_1")
	}
	return typeName
}

func allowsFiveMClientAPISet(apiSet string) bool {
	apiSet = strings.ToLower(strings.TrimSpace(apiSet))
	switch apiSet {
	case "", "client", "shared", "all":
		return true
	default:
		return false
	}
}

func allowsFiveMServerAPISet(apiSet string) bool {
	apiSet = strings.ToLower(strings.TrimSpace(apiSet))
	switch apiSet {
	case "", "server", "shared", "all":
		return true
	default:
		return false
	}
}

func matchesFiveMGame(value, target string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	target = strings.ToLower(strings.TrimSpace(target))
	if value == "" || value == target || value == "common" || value == "all" {
		return true
	}
	return false
}

func isFiveMNonReturnPointerNative(name string) bool {
	_, ok := fiveMNonReturnPointerNatives[strings.ToUpper(strings.TrimSpace(name))]
	return ok
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

var fiveMNonReturnPointerNatives = map[string]struct{}{
	"DELETE_ENTITY":                           {},
	"SET_ENTITY_AS_NO_LONGER_NEEDED":          {},
	"SET_PED_AS_NO_LONGER_NEEDED":             {},
	"DELETE_PED":                              {},
	"REMOVE_PED_ELEGANTLY":                    {},
	"SET_VEHICLE_AS_NO_LONGER_NEEDED":         {},
	"DELETE_MISSION_TRAIN":                    {},
	"DELETE_VEHICLE":                          {},
	"SET_MISSION_TRAIN_AS_NO_LONGER_NEEDED":   {},
	"DELETE_OBJECT":                           {},
	"SET_OBJECT_AS_NO_LONGER_NEEDED":          {},
	"SET_PLAYER_WANTED_CENTRE_POSITION":       {},
	"_START_SHAPE_TEST_SURROUNDING_COORDS":    {},
	"REMOVE_BLIP":                             {},
	"SET_BIT":                                 {},
	"CLEAR_BIT":                               {},
	"SET_SCALEFORM_MOVIE_AS_NO_LONGER_NEEDED": {},
	"DELETE_ROPE":                             {},
	"DOES_ROPE_EXIST":                         {},
	"CLEAR_SEQUENCE_TASK":                     {},
}
