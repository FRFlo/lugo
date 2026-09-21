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
	// A row records whether a pattern suffix matches each candidate suffix.
	// Keeping only three rows bounds memory by the candidate length while the
	// bottom-up evaluation visits each state once.
	rowAfter := make([]bool, len(candidate)+1)
	rowAfter[len(candidate)] = true
	rowTwoAfter := make([]bool, len(candidate)+1)
	row := make([]bool, len(candidate)+1)

	for patternIndex := len(pattern) - 1; patternIndex >= 0; patternIndex-- {
		switch pattern[patternIndex] {
		case '*':
			rowToSkip := rowAfter
			matchAny := false
			if patternIndex+1 < len(pattern) && pattern[patternIndex+1] == '*' {
				rowToSkip = rowTwoAfter
				matchAny = true
			}
			row[len(candidate)] = rowToSkip[len(candidate)]
			for candidateIndex := len(candidate) - 1; candidateIndex >= 0; candidateIndex-- {
				row[candidateIndex] = rowToSkip[candidateIndex] ||
					(matchAny || candidate[candidateIndex] != '/') && row[candidateIndex+1]
			}
		case '?':
			row[len(candidate)] = false
			for candidateIndex := len(candidate) - 1; candidateIndex >= 0; candidateIndex-- {
				row[candidateIndex] = candidate[candidateIndex] != '/' && rowAfter[candidateIndex+1]
			}
		default:
			row[len(candidate)] = false
			for candidateIndex := len(candidate) - 1; candidateIndex >= 0; candidateIndex-- {
				row[candidateIndex] = pattern[patternIndex] == candidate[candidateIndex] && rowAfter[candidateIndex+1]
			}
		}
		row, rowTwoAfter, rowAfter = rowTwoAfter, rowAfter, row
	}
	return rowAfter[0]
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
