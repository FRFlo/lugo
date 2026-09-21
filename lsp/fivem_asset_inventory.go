package lsp

import "strings"

// FiveMAssetIssueKind identifies a manifest asset inventory problem.
type FiveMAssetIssueKind string

const (
	FiveMAssetIssueMissing       FiveMAssetIssueKind = "missing"
	FiveMAssetIssueEmptyGlob     FiveMAssetIssueKind = "empty-glob"
	FiveMAssetIssuePathTraversal FiveMAssetIssueKind = "path-traversal"
	FiveMAssetIssueCaseMismatch  FiveMAssetIssueKind = "case-mismatch"
)

// FiveMAssetIssue is deliberately independent of diagnostics. It is the small
// result of checking manifest paths against a caller-provided path inventory.
type FiveMAssetIssue struct {
	Kind FiveMAssetIssueKind
	Path string
}

// FiveMAssetInventory contains relative paths known to exist in a resource.
// It does not read files or directories and therefore remains useful to the
// workspace indexer's lightweight inventory.
type FiveMAssetInventory struct {
	Paths []string
}

func NewFiveMAssetInventory(paths []string) FiveMAssetInventory {
	inventory := FiveMAssetInventory{Paths: make([]string, 0, len(paths))}
	for _, path := range paths {
		inventory.Paths = append(inventory.Paths, normalizeFiveMAssetPath(path))
	}
	return inventory
}

func normalizeFiveMAssetPath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

func fiveMAssetPathTraversal(path string) bool {
	for _, part := range strings.Split(normalizeFiveMAssetPath(path), "/") {
		if part == ".." {
			return true
		}
	}
	return strings.HasPrefix(path, "/") || (len(path) > 1 && path[1] == ':')
}

func fiveMAssetIsGlob(path string) bool {
	return strings.ContainsAny(path, "*?")
}

// fiveMAssetMatchGlob is intentionally case-sensitive. The general LSP glob
// matcher is user-friendly and case-insensitive, which is not appropriate for
// filesystem asset validation.
func fiveMAssetMatchGlob(pattern, candidate string) bool {
	if pattern == "" {
		return candidate == ""
	}
	if pattern[0] == '*' {
		if len(pattern) > 1 && pattern[1] == '*' {
			for i := 0; i <= len(candidate); i++ {
				if fiveMAssetMatchGlob(pattern[2:], candidate[i:]) {
					return true
				}
			}
			return false
		}
		for i := 0; i <= len(candidate) && (i == 0 || candidate[i-1] != '/'); i++ {
			if fiveMAssetMatchGlob(pattern[1:], candidate[i:]) {
				return true
			}
		}
		return false
	}
	if pattern[0] == '?' {
		return len(candidate) > 0 && candidate[0] != '/' && fiveMAssetMatchGlob(pattern[1:], candidate[1:])
	}
	return len(candidate) > 0 && pattern[0] == candidate[0] && fiveMAssetMatchGlob(pattern[1:], candidate[1:])
}

func (inventory FiveMAssetInventory) Validate(res *FiveMResource) []FiveMAssetIssue {
	if res == nil {
		return nil
	}
	var issues []FiveMAssetIssue
	check := func(path string) {
		path = normalizeFiveMAssetPath(path)
		if path == "" {
			issues = append(issues, FiveMAssetIssue{Kind: FiveMAssetIssueEmptyGlob, Path: path})
			return
		}
		if strings.HasPrefix(path, "@") {
			return // cross-resource assets are outside this local inventory
		}
		if fiveMAssetPathTraversal(path) {
			issues = append(issues, FiveMAssetIssue{Kind: FiveMAssetIssuePathTraversal, Path: path})
			return
		}
		glob := fiveMAssetIsGlob(path)
		matched, folded := false, false
		for _, candidate := range inventory.Paths {
			candidate = normalizeFiveMAssetPath(candidate)
			if (glob && fiveMAssetMatchGlob(path, candidate)) || (!glob && path == candidate) {
				matched = true
				break
			}
			if (glob && fiveMAssetMatchGlob(strings.ToLower(path), strings.ToLower(candidate))) || (!glob && strings.EqualFold(path, candidate)) {
				folded = true
			}
		}
		if matched {
			return
		}
		if folded {
			issues = append(issues, FiveMAssetIssue{Kind: FiveMAssetIssueCaseMismatch, Path: path})
		} else if glob {
			issues = append(issues, FiveMAssetIssue{Kind: FiveMAssetIssueEmptyGlob, Path: path})
		} else {
			issues = append(issues, FiveMAssetIssue{Kind: FiveMAssetIssueMissing, Path: path})
		}
	}

	if res.UIPage != "" {
		check(res.UIPage)
	}
	for _, path := range res.SharedGlobs {
		check(path)
	}
	for _, path := range res.ClientGlobs {
		check(path)
	}
	for _, path := range res.ServerGlobs {
		check(path)
	}
	return issues
}
