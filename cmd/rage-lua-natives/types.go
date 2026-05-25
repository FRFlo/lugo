package main

import (
	"regexp"
	"strings"
)

type GameType int

const (
	GameGTA GameType = iota
	GameCFX
)

type NativeParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type NativeDefinition struct {
	Name        string        `json:"name"`
	Params      []NativeParam `json:"params"`
	Results     string        `json:"results"`
	Description string        `json:"description"`
	Examples    []any         `json:"examples"`
	Hash        string        `json:"hash"`
	Ns          string        `json:"ns"`
	Jhash       string        `json:"jhash"`
	ManualHash  bool          `json:"manualHash"`
	ReturnType  string        `json:"return_type"`
	Comment     string        `json:"comment"`
	Apiset      string        `json:"apiset"`
	Aliases     []string      `json:"aliases"`
	OldNames    []string      `json:"old_names"`
}

var nativeNameWord = regexp.MustCompile(`_([a-z])`)

func (g GameType) String() string {
	switch g {
	case GameCFX:
		return "cfx"
	default:
		return "gtav"
	}
}

func (g GameType) JSONURL() string {
	switch g {
	case GameCFX:
		return "https://static.cfx.re/natives/natives_cfx.json"
	default:
		return "https://static.cfx.re/natives/natives.json"
	}
}

func (g GameType) DocsURL() string {
	return "https://docs.fivem.net/natives/?_"
}

func parseGameTypes(raw string) []GameType {
	if strings.TrimSpace(raw) == "" {
		return []GameType{GameGTA, GameCFX}
	}

	parts := strings.Split(raw, ",")
	var games []GameType
	for _, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "gta", "gtav", "gtav5", "gta5":
			games = append(games, GameGTA)
		case "cfx", "cfx-native", "cfxnative":
			games = append(games, GameCFX)
		}
	}

	return games
}

func nativeName(native NativeDefinition, fallback string) string {
	base := native.Name
	if base == "" {
		base = fallback
	}

	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, "0x", "n_0x")
	base = nativeNameWord.ReplaceAllStringFunc(base, func(bit string) string {
		return strings.ToUpper(strings.TrimPrefix(bit, "_"))
	})

	if len(base) > 0 && base[0] >= 'a' && base[0] <= 'z' {
		base = strings.ToUpper(base[:1]) + base[1:]
	}

	return base
}

func fieldToReplace(field string) string {
	switch field {
	case "end", "repeat", "local":
		return "_" + field
	default:
		return field
	}
}

func getNativeType(typ string, input bool) string {
	typ = strings.ToLower(strings.TrimSpace(typ))

	switch typ {
	case "vector3", "string", "void":
		return typ
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
