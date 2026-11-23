package lsp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMResourceSelection models only statically known startup commands. If a
// command or dependency cannot be resolved, complete is false and inference
// falls back to the workspace rather than excluding possible handlers.
type fiveMResourceSelection struct {
	complete bool
	active   map[string]int // resource root URI -> first startup command
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *Server) fiveMSelectionWorkspace(doc *Document) string {
	profile := s.getDocumentFiveMProfile(doc)
	if profile.ResourceRoot == "" {
		return ""
	}
	resourcePath := s.uriToPath(profile.ResourceRoot)
	best := ""
	for _, uri := range s.WorkspaceFolders {
		root := s.uriToPath(uri)
		if root != "" && pathWithin(root, resourcePath) && len(root) > len(best) {
			best = root
		}
	}
	return best
}

func (s *Server) fiveMSelectionConfigPath(workspace string) string {
	if s.FiveMServerConfig != "" {
		if filepath.IsAbs(s.FiveMServerConfig) {
			return filepath.Clean(s.FiveMServerConfig)
		}
		return filepath.Join(workspace, s.FiveMServerConfig)
	}
	for dir := filepath.Clean(workspace); ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "server.cfg")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}

func (s *Server) buildFiveMResourceSelection(workspace string) *fiveMResourceSelection {
	selection := &fiveMResourceSelection{active: make(map[string]int)}
	path := s.fiveMSelectionConfigPath(workspace)
	if path == "" || s.FiveMResourceGraph == nil {
		return selection
	}
	config := readFiveMServerConfig(path)
	if !config.Complete {
		return selection
	}
	byName := make(map[string][]*FiveMResource)
	var resources []*FiveMResource
	for root, node := range s.FiveMResourceGraph.ByRoot {
		if node == nil || node.Resource == nil || !pathWithin(workspace, s.uriToPath(root)) {
			continue
		}
		resource := node.Resource
		resources = append(resources, resource)
		names := append([]string{resource.Name}, resource.Provides...)
		seenNames := make(map[string]struct{}, len(names))
		for _, name := range names {
			key := strings.ToLower(name)
			if key == "" {
				continue
			}
			if _, exists := seenNames[key]; exists {
				continue
			}
			seenNames[key] = struct{}{}
			byName[key] = append(byName[key], resource)
		}
	}
	selection.complete = true
	activeStack := make(map[string]bool)
	var start func(*FiveMResource, int)
	start = func(resource *FiveMResource, position int) {
		if !selection.complete || resource == nil {
			return
		}
		if _, exists := selection.active[resource.RootURI]; exists {
			return
		}
		if activeStack[resource.RootURI] {
			selection.complete = false
			return
		}
		activeStack[resource.RootURI] = true
		defer delete(activeStack, resource.RootURI)
		for _, dependency := range resource.Dependencies {
			if strings.HasPrefix(dependency, "/") {
				continue
			}
			matches := byName[strings.ToLower(dependency)]
			if len(matches) != 1 {
				selection.complete = false
				return
			}
			start(matches[0], position)
		}
		if selection.complete {
			selection.active[resource.RootURI] = position
		}
	}
	for pos, command := range config.Commands {
		var targets []*FiveMResource
		if strings.HasPrefix(command.Argument, "[") && strings.HasSuffix(command.Argument, "]") {
			for _, resource := range resources {
				for _, segment := range strings.Split(filepath.ToSlash(s.uriToPath(resource.RootURI)), "/") {
					if segment == command.Argument {
						targets = append(targets, resource)
						break
					}
				}
			}
			if len(targets) == 0 {
				selection.complete = false
				break
			}
		} else {
			matches := byName[strings.ToLower(command.Argument)]
			if len(matches) != 1 {
				selection.complete = false
				break
			}
			targets = matches
		}
		for _, target := range targets {
			if command.Name == "stop" || command.Name == "restart" {
				// FiveM may also affect active dependents. Do not guess their
				// state when a stopped resource is a dependency of another.
				for _, other := range resources {
					if other.RootURI == target.RootURI {
						continue
					}
					if _, active := selection.active[other.RootURI]; !active {
						continue
					}
					for _, dependency := range other.Dependencies {
						matches := byName[strings.ToLower(dependency)]
						if len(matches) == 1 && matches[0].RootURI == target.RootURI {
							selection.complete = false
						}
					}
				}
				delete(selection.active, target.RootURI)
			}
			if command.Name != "stop" {
				start(target, pos)
			}
		}
		if !selection.complete {
			break
		}
	}
	return selection
}

func (s *Server) fiveMResourceSelectionFor(doc *Document) *fiveMResourceSelection {
	workspace := s.fiveMSelectionWorkspace(doc)
	if workspace == "" {
		return nil
	}
	s.fiveMEventsNamed("")
	index := &s.fiveMEventIndex
	index.mu.Lock()
	defer index.mu.Unlock()
	if selection, ok := index.selections[workspace]; ok {
		return selection
	}
	selection := s.buildFiveMResourceSelection(workspace)
	index.selections[workspace] = selection
	return selection
}

// fiveMEventResourcesCompatible applies an authoritative static configuration
// only when it lists the source resource. Unlisted sources and incomplete
// configs retain the workspace-based fallback.
func (s *Server) fiveMEventResourcesCompatible(source, target *Document, callID ast.NodeID, kind FiveMEventKind) bool {
	if source == nil || target == nil {
		return false
	}
	selection := s.fiveMResourceSelectionFor(source)
	if selection == nil || !selection.complete {
		return true
	}
	sourceRoot := s.getDocumentFiveMProfile(source).ResourceRoot
	targetRoot := s.getDocumentFiveMProfile(target).ResourceRoot
	sourceOrder, ok := selection.active[sourceRoot]
	if !ok {
		return true
	}
	targetOrder, ok := selection.active[targetRoot]
	if !ok {
		return false
	}
	// Resource order only constrains an unconditional top-level local call.
	// Calls inside functions, threads or conditional blocks may run later.
	if kind == FiveMEventTriggerLocal && sourceRoot != targetRoot && source.Tree != nil && callID != ast.InvalidNode && int(callID) < len(source.Tree.Nodes) {
		parent := source.Tree.Nodes[callID].Parent
		if parent != ast.InvalidNode && int(parent) < len(source.Tree.Nodes) && source.Tree.Nodes[parent].Kind == ast.KindBlock {
			blockParent := source.Tree.Nodes[parent].Parent
			if blockParent != ast.InvalidNode && int(blockParent) < len(source.Tree.Nodes) && source.Tree.Nodes[blockParent].Kind == ast.KindFile {
				return targetOrder <= sourceOrder
			}
		}
	}
	return true
}

// Configuration changes can introduce previously invisible providers, so
// invalidate all event documents and their tracked type dependents.
func (s *Server) invalidateFiveMEventConfiguration() {
	var uris []string
	for uri, doc := range s.Documents {
		if doc == nil || doc.Tree == nil || len(doc.FiveMEvents) == 0 {
			continue
		}
		doc.TypeCache.Reset(len(doc.Tree.Nodes))
		clear(doc.Inferring)
		uris = append(uris, uri)
	}
	s.invalidateMetatableIndexes(uris...)
}
