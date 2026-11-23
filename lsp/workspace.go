package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

const (
	embeddedStdlibURIPrefix = "lugo-stdlib/"
	// builtinFiveMEventsURI is a stable, human-readable virtual document URI.
	// It is also used by workspace symbols, keeping event navigation consistent.
	builtinFiveMEventsURI = "builtin://fivem/events"
)

type IndexJob struct {
	Uri            string
	Path           string
	ModTime        time.Time
	IsStd          bool
	ExistingTree   *ast.Tree
	ExistingSource []byte
	Doc            *Document
}

type IndexResult struct {
	Job       *IndexJob
	Uri       string
	Source    []byte
	Tree      *ast.Tree
	Errors    []parser.ParseError
	Resolver  *semantic.Resolver
	Err       error
	Unchanged bool
}

type IgnorePattern struct {
	MatchFallback string
	HasSuffix     string
	HasPrefix     string
	ContainsPath  string
	SuffixPath    string
}

func (s *Server) handleDidOpen(req Request) {
	var params DidOpenTextDocumentParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	if s.documentVersions == nil {
		s.documentVersions = make(map[string]int)
	}
	if version, ok := s.documentVersions[uri]; ok && params.TextDocument.Version <= version {
		return
	}
	s.documentVersions[uri] = params.TextDocument.Version
	s.OpenFiles[uri] = true
	if isNUIAssetURI(uri) {
		s.updateNUIAssetDocument(uri, []byte(params.TextDocument.Text))
		// Assets participate in NUI contract diagnostics, but never enter the Lua
		// parser or receive Lua diagnostics.
		s.publishWorkspaceDiagnostics()
		return
	}
	if s.isIgnoredURI(uri) {
		if s.ignoredOpenSources == nil {
			s.ignoredOpenSources = make(map[string][]byte)
		}
		if s.contentExceedsMaxFileSize(params.TextDocument.Text) {
			s.ignoredOpenSources[uri] = nil
		} else {
			s.ignoredOpenSources[uri] = []byte(params.TextDocument.Text)
		}

		return
	}

	if s.contentExceedsMaxFileSize(params.TextDocument.Text) {
		if s.oversizedOpenFiles == nil {
			s.oversizedOpenFiles = make(map[string]bool)
		}
		s.oversizedOpenFiles[uri] = true
		s.publishFileSizeDiagnostic(uri)
		return
	}

	_, hadDocument := s.Documents[uri]
	needsRepublish := s.updateDocument(uri, []byte(params.TextDocument.Text))
	if !hadDocument && documentHasPublicFacts(s.Documents[uri]) {
		needsRepublish = true
	}

	if needsRepublish {
		s.publishWorkspaceDiagnostics()
	} else {
		s.publishDiagnostics(uri)
	}

	s.Log.Debugf("Opened document: %s\n", uri)
}

func (s *Server) handleDidChange(req Request) {
	var params DidChangeTextDocumentParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	if s.documentVersions == nil {
		s.documentVersions = make(map[string]int)
	}
	if version, ok := s.documentVersions[uri]; ok && params.TextDocument.Version <= version {
		return
	}
	s.documentVersions[uri] = params.TextDocument.Version
	if s.isIgnoredURI(uri) {
		if len(params.ContentChanges) > 0 {
			if s.ignoredOpenSources == nil {
				s.ignoredOpenSources = make(map[string][]byte)
			}
			if s.contentExceedsMaxFileSize(params.ContentChanges[0].Text) {
				s.ignoredOpenSources[uri] = nil
			} else {
				s.ignoredOpenSources[uri] = []byte(params.ContentChanges[0].Text)
			}
		}
		if _, ok := s.Documents[uri]; ok {
			s.clearDocument(uri)
		}
		return
	}

	if len(params.ContentChanges) > 0 {
		if isNUIAssetURI(uri) {
			s.updateNUIAssetDocument(uri, []byte(params.ContentChanges[0].Text))
			s.publishWorkspaceDiagnostics()
			return
		}
		if s.contentExceedsMaxFileSize(params.ContentChanges[0].Text) {
			if s.oversizedOpenFiles == nil {
				s.oversizedOpenFiles = make(map[string]bool)
			}
			s.oversizedOpenFiles[uri] = true
			s.publishFileSizeDiagnostic(uri)
			return
		}
		wasOversized := s.oversizedOpenFiles[uri]
		delete(s.oversizedOpenFiles, uri)
		if doc := s.Documents[uri]; !wasOversized && doc != nil && doc.Tree != nil && doc.Tree.Source != nil && string(doc.Tree.Source) == params.ContentChanges[0].Text {
			return
		}

		_, hadDocument := s.Documents[uri]
		needsRepublish := s.updateDocument(uri, []byte(params.ContentChanges[0].Text))
		if !hadDocument && documentHasPublicFacts(s.Documents[uri]) {
			needsRepublish = true
		}

		if needsRepublish {
			s.publishWorkspaceDiagnostics()
		} else {
			s.publishDiagnostics(uri)
		}

		s.Log.Debugf("Updated document: %s\n", uri)
	}
}

func isNUIAssetURI(uri string) bool {
	switch strings.ToLower(filepath.Ext(uri)) {
	case ".html", ".js", ".ts", ".jsx", ".tsx":
		return true
	default:
		return false
	}
}

func documentHasPublicFacts(doc *Document) bool {
	return doc != nil && (len(doc.ExportedGlobalDefs) != 0 || len(doc.FiveMLuaExports) != 0 || len(doc.FiveMEvents) != 0)
}

// updateNUIAssetDocument keeps the latest open-buffer bytes available to the
// NUI scanner without treating JavaScript/HTML as Lua source.
func (s *Server) updateNUIAssetDocument(uri string, source []byte) {
	doc := s.Documents[uri]
	if doc == nil {
		doc = &Document{Server: s, URI: uri}
		doc.Path = s.uriToPath(uri)
		doc.LowerPath = strings.ToLower(doc.Path)
		doc.Dir = filepath.Dir(doc.Path)
		doc.ModuleName = s.computeModuleName(uri, doc.Path, doc.LowerPath)
		s.Documents[uri] = doc
	}
	if doc.Tree == nil {
		doc.Tree = ast.NewTree(source)
	} else {
		doc.Tree.Reset(source)
	}
	doc.IsWorkspace = false
}

func (s *Server) handleDidClose(req Request) {
	var params DidCloseTextDocumentParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)
	if isNUIAssetURI(uri) {
		delete(s.OpenFiles, uri)
		delete(s.documentVersions, uri)
		if source, err := os.ReadFile(s.uriToPath(uri)); err == nil {
			s.updateNUIAssetDocument(uri, source)
		} else {
			s.clearDocument(uri)
		}
		s.publishWorkspaceDiagnostics()
		return
	}

	delete(s.OpenFiles, uri)
	delete(s.ignoredOpenSources, uri)
	delete(s.oversizedOpenFiles, uri)
	delete(s.documentVersions, uri)
	// Evict caches for documents that are no longer open to reclaim memory
	evictClosedDocumentCaches(s)

	path := s.uriToPath(uri)
	if path != "" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			s.clearDocument(uri)
		}
	} else if strings.HasPrefix(uri, "untitled:") {
		s.clearDocument(uri)
	}

	s.Log.Debugf("Closed document: %s\n", uri)
}

func (s *Server) handleDidChangeConfiguration(req Request) {
	var params DidChangeConfigurationParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	previousMaxErrors := s.MaxParseErrors
	needsReindex, needsRepublish := s.applyInitializationOptions(params.Settings)
	if previousMaxErrors != s.MaxParseErrors {
		s.updateParseErrorLimit(previousMaxErrors)
	}

	if needsReindex {
		s.refreshWorkspace()
	} else if needsRepublish {
		s.publishWorkspaceDiagnostics()
	}
}

// updateParseErrorLimit changes only syntax diagnostics. The parser's limit
// affects error collection, not its AST, so symbols do not need reindexing.
func (s *Server) updateParseErrorLimit(previous int) {
	current := s.MaxParseErrors
	for uri, doc := range s.Documents {
		if doc == nil || doc.Tree == nil {
			continue
		}
		if current > 0 && len(doc.Errors) > current {
			doc.Errors = doc.Errors[:current]
			continue
		}
		if previous <= 0 || (current > 0 && current <= previous) || len(doc.Errors) < previous {
			continue
		}
		source := doc.Tree.Source
		if source == nil {
			var err error
			if strings.HasPrefix(uri, embeddedStdlibURIPrefix) {
				source, err = stdlibFS.ReadFile("stdlib/" + strings.TrimPrefix(uri, embeddedStdlibURIPrefix))
			} else {
				source, err = os.ReadFile(doc.Path)
			}
			if err != nil {
				s.Log.Warnf("Cannot reload document to update parser errors %s: %v\n", uri, err)
				continue
			}
			// The source may have changed on disk since it was evicted. Rebuild
			// this document so its AST and symbols match the new diagnostics.
			s.updateDocument(uri, source)
			continue
		}
		tree := ast.NewTree(source)
		tree.SetPositionEncoding(s.positionEncoding)
		p := parser.New(source, tree, current)
		p.Parse()
		doc.Errors = append(doc.Errors[:0], p.Errors...)
	}
}

func (s *Server) handleDidChangeWatchedFiles(req Request) {
	var params DidChangeWatchedFilesParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		telemetryCtx := WithTraceContext(context.Background(), s.trace)
		RecordTelemetry(telemetryCtx, "lugo.watched_files.refresh_degraded", map[string]any{"reason": "invalid_params"})
		return
	}

	var (
		needsWorkspaceRepublish bool
		moduleSetChanged        bool
		singleRepublish         []string
		statFailures            int
		readFailures            int
		oversizedFiles          int
	)

	for _, change := range params.Changes {
		uri := s.normalizeURI(change.URI)

		if s.isIgnoredURI(uri) {
			continue
		}
		if strings.EqualFold(filepath.Ext(s.uriToPath(uri)), ".cfg") {
			s.invalidateFiveMEventConfiguration()
			needsWorkspaceRepublish = true
			continue
		}

		switch change.Type {
		case 1, 2: // Created, Changed
			if !s.OpenFiles[uri] {
				// Files changed outside the editor no longer have a meaningful
				// LSP document version; discard it before re-indexing the source.
				delete(s.documentVersions, uri)
				path := s.uriToPath(uri)

				stat, statErr := os.Stat(path)
				if statErr != nil {
					statFailures++
				}
				if statErr == nil && stat.Size() > s.MaxFileSize {
					oversizedFiles++
					continue
				}

				if b, err := os.ReadFile(path); err == nil {
					needsRepublish := false
					if isNUIAssetURI(uri) {
						s.updateNUIAssetDocument(uri, b)
						needsWorkspaceRepublish = true
					} else if doc := s.Documents[uri]; doc != nil && doc.Tree != nil && doc.Tree.Source != nil && bytes.Equal(doc.Tree.Source, b) {
						if statErr == nil {
							doc.ModTime = stat.ModTime()
						}
						continue
					} else {
						_, hadDocument := s.Documents[uri]
						needsRepublish = s.updateDocument(uri, b)
						if !hadDocument && documentHasPublicFacts(s.Documents[uri]) {
							needsRepublish = true
						}
						if !hadDocument && s.Documents[uri] != nil {
							moduleSetChanged = true
						}

					}

					if statErr == nil && s.Documents[uri] != nil {
						s.Documents[uri].ModTime = stat.ModTime()
					}

					if s.isWorkspaceURI(uri) {
						if needsRepublish {
							needsWorkspaceRepublish = true
						} else {
							singleRepublish = append(singleRepublish, uri)
						}
					}
				} else {
					readFailures++
				}
			}
		case 3: // Deleted
			if s.Documents[uri] != nil {
				moduleSetChanged = true
			}
			delete(s.documentVersions, uri)
			// If a manifest file is deleted, evict its FiveM resource state before
			// removing the document from the workspace cache.
			if strings.HasSuffix(uri, "/fxmanifest.lua") || strings.HasSuffix(uri, "/__resource.lua") {
				removeFiveMResource(s, URI(uri))
			}

			if isNUIAssetURI(uri) || strings.HasSuffix(uri, ".lua") || strings.HasSuffix(uri, "/fxmanifest.lua") || strings.HasSuffix(uri, "/__resource.lua") {
				needsWorkspaceRepublish = true
			}

			// A removed Lua source can change profile, events, and cross-file
			// contracts for every sibling in its resource. Resolve scope from the
			// retained document before clearDocument removes that context.
			if strings.HasSuffix(uri, ".lua") && !strings.HasSuffix(uri, "/fxmanifest.lua") && !strings.HasSuffix(uri, "/__resource.lua") {
				resourceRoot := ""
				if deletedDoc := s.Documents[uri]; deletedDoc != nil {
					resourceRoot = s.getDocResourceRoot(deletedDoc)
				}
				deletedPath := filepath.Clean(s.uriToPath(uri))
				if resolved, err := filepath.EvalSymlinks(deletedPath); err == nil {
					deletedPath = resolved
				}
				resourceRootPath := ""
				for root := range s.FiveMResourceGraph.ByRoot {
					rootPath := filepath.Clean(s.uriToPath(root))
					if resolved, err := filepath.EvalSymlinks(rootPath); err == nil {
						rootPath = resolved
					}
					rel, err := filepath.Rel(rootPath, deletedPath)
					if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(rootPath) > len(resourceRootPath) {
						resourceRoot = root
						resourceRootPath = rootPath
					}
				}
				for dURI, d := range s.Documents {
					if d == nil || dURI == uri {
						continue
					}
					inResource := resourceRoot != "" && s.getDocResourceRoot(d) == resourceRoot
					if !inResource && resourceRoot != "" {
						path := filepath.Clean(s.uriToPath(dURI))
						if resolved, err := filepath.EvalSymlinks(path); err == nil {
							path = resolved
						}
						rel, err := filepath.Rel(resourceRootPath, path)
						inResource = err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
					}
					if inResource {
						d.FiveMProfile = FiveMExecutionProfile{}
						d.FiveMProfileCached = false
						d.FiveMEvents = d.FiveMEvents[:0]
					}
				}
			}

			// Invalidate dependent resource documents before removing the source
			// document; clearDocument evicts its cached resource context.
			s.clearDocument(uri)
		}
	}

	if moduleSetChanged {
		needsWorkspaceRepublish = s.rebindPendingRequireFields() || needsWorkspaceRepublish
	}

	if statFailures > 0 || readFailures > 0 || oversizedFiles > 0 {
		telemetryCtx := WithTraceContext(context.Background(), s.trace)
		RecordTelemetry(telemetryCtx, "lugo.watched_files.refresh_degraded", map[string]any{
			"reason":          "file_refresh_incomplete",
			"stat_failures":   boundedWatchedFileCount(statFailures),
			"read_failures":   boundedWatchedFileCount(readFailures),
			"oversized_files": boundedWatchedFileCount(oversizedFiles),
		})
	}

	if needsWorkspaceRepublish {
		s.publishWorkspaceDiagnostics()
	} else {
		for _, uri := range singleRepublish {
			s.publishDiagnostics(uri)
		}
	}
}

// Rebind only references to require() modules after files are added or removed.
// Closed documents whose source was evicted are reparsed when reopened.
func (s *Server) rebindPendingRequireFields() bool {
	changed := false
	for uri, doc := range s.Documents {
		if doc == nil || doc.Tree == nil || doc.Tree.Source == nil || doc.Resolver == nil {
			continue
		}
		for i, field := range doc.Resolver.PendingFields {
			var module string
			if field.ReceiverDef != ast.InvalidNode {
				module = s.getRequireModName(doc, doc.getAssignedValue(field.ReceiverDef))
			} else if int(field.PropNodeID) < len(doc.Tree.Nodes) {
				parentID := doc.Tree.Nodes[field.PropNodeID].Parent
				if parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
					parent := doc.Tree.Nodes[parentID]
					if parent.Kind == ast.KindMemberExpr || parent.Kind == ast.KindMethodCall {
						module = s.getRequireModName(doc, parent.Left)
					}
				}
			}
			if module == "" {
				continue
			}
			hash := ast.HashBytes(field.ReceiverName)
			if target := s.resolveModule(uri, module); target != nil {
				hash = ast.HashBytesConcat([]byte("module:"), nil, []byte(target.URI))
			}
			if field.ReceiverHash != hash {
				doc.Resolver.PendingFields[i].ReceiverHash = hash
				changed = true
			}
		}
	}
	return changed
}

func boundedWatchedFileCount(count int) int {
	if count > 1000 {
		return 1000
	}
	return count
}

func (s *Server) handleReindex(req Request) {
	s.refreshWorkspace()

	WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: "ok"})
}

// refreshWorkspace must run through serialized workspace dispatch. Its workers
// may reset published document trees during the parse phase, so callers must
// not launch document readers or another refresh concurrently. All consumers
// run only after the result channel closes and the workers stop mutating trees.
func (s *Server) refreshWorkspace() {
	s.Log.Println("Starting workspace re-index...")

	telemetryCtx := WithTraceContext(context.Background(), s.trace)
	RecordTelemetry(telemetryCtx, "lugo.workspace_index.start", map[string]any{})
	start := time.Now()

	var (
		total              int
		indexed            int
		unchanged          int
		failed             int
		moduleNamesChanged bool
	)
	defer func() {
		properties := map[string]any{
			"duration_ms":  time.Since(start).Milliseconds(),
			"indexed":      indexed,
			"unchanged":    unchanged,
			"failed":       failed,
			"bytes_bucket": workspaceIndexBytesBucket(total),
		}
		if r := recover(); r != nil {
			// Do not include the recovered value: panic values can contain source,
			// paths, or raw errors. Re-panic to preserve refreshWorkspace semantics.
			properties["outcome"] = "panic"
			RecordTelemetry(telemetryCtx, "lugo.workspace_index.finish", properties)
			panic(r)
		}
		properties["outcome"] = "success"
		RecordTelemetry(telemetryCtx, "lugo.workspace_index.finish", properties)
	}()

	s.IsIndexing = true
	// Workspace refresh finalizes many documents serially. Their metatable and
	// type-cache invalidations can be coalesced because no document readers run
	// until refreshWorkspace publishes diagnostics below.
	s.metatableInvalidationBatch = make(map[string]struct{})
	defer func() { s.metatableInvalidationBatch = nil }()

	if s.workDoneProgressSupport {
		// Request client to create a work done progress token.
		WriteMessage(s.Writer, OutgoingRequest{
			RPC:    "2.0",
			ID:     nextOutgoingRequestID(),
			Method: "window/workDoneProgress/create",
			Params: map[string]string{"token": "indexing"},
		})

		// Notify client that indexing has started.
		s.sendProgressNotification("indexing", WorkDoneProgressBegin{
			Kind:        "begin",
			Title:       "Lugo: Preparing workspace",
			Cancellable: false,
		})
	}

	if s.activeURIs == nil {
		s.activeURIs = make(map[string]bool, len(s.Documents))
	} else {
		clear(s.activeURIs)
	}

	oldBindings := s.snapshotFiveMBindingsUnder("")
	oldResources := make(map[string]*FiveMResource)
	if s.FiveMResourceGraph != nil {
		for root, node := range s.FiveMResourceGraph.ByRoot {
			if node != nil {
				oldResources[root] = node.Resource
			}
		}
	}

	// Graph-based state is authoritative; no local maps to clear
	if s.FiveMResourceGraph != nil {
		s.FiveMResourceGraph.Clear()
	}
	// Keep the editor's unsaved contents while excluding open files from the
	// index. The client does not send didOpen again when an ignore is removed.
	for uri := range s.OpenFiles {
		if s.isIgnoredURI(uri) {
			if s.ignoredOpenSources == nil {
				s.ignoredOpenSources = make(map[string][]byte)
			}
			if doc := s.Documents[uri]; doc != nil {
				moduleNamesChanged = true
				if !s.oversizedOpenFiles[uri] && (s.MaxFileSize <= 0 || int64(len(doc.Source())) <= s.MaxFileSize) {
					s.ignoredOpenSources[uri] = doc.Source()
				} else {
					s.ignoredOpenSources[uri] = nil
				}
				s.clearDocument(uri)
			} else if source, exists := s.ignoredOpenSources[uri]; !exists || (s.MaxFileSize > 0 && int64(len(source)) > s.MaxFileSize) {
				s.ignoredOpenSources[uri] = nil
			}
		}
	}

	var pendingJobs []*IndexJob
	workspaceScanComplete := true
	if s.visitedDirs == nil {
		s.visitedDirs = make(map[string]bool, 256)
	} else {
		clear(s.visitedDirs)
	}

	s.indexEmbeddedStdlib(&pendingJobs, &unchanged)

	for _, libPath := range s.LibraryPaths {
		s.Log.Printf("Indexing external library: %s\n", libPath)

		workspaceScanComplete = s.indexWorkspace(libPath, &pendingJobs, &unchanged, &failed) && workspaceScanComplete
	}

	for _, wf := range s.WorkspaceFolders {
		s.Log.Printf("Indexing workspace folder: %s\n", wf)

		workspaceScanComplete = s.indexWorkspace(wf, &pendingJobs, &unchanged, &failed) && workspaceScanComplete
	}
	pendingJobs = deduplicateIndexJobs(pendingJobs)

	s.sendProgressNotification("indexing", WorkDoneProgressReport{
		Kind:    "report",
		Message: fmt.Sprintf("Found %d files to process...", len(pendingJobs)),
	})

	caches := s.openWorkspaceCaches()
	jobCaches, nestedCaches := nestedRepositoryCaches(caches, pendingJobs)
	defer func() {
		closed := make(map[*parseCache]bool, len(caches)+len(nestedCaches))
		for _, root := range caches {
			if !closed[root.cache] {
				root.cache.close()
				closed[root.cache] = true
			}
		}
		for _, cache := range nestedCaches {
			if !closed[cache] {
				cache.close()
				closed[cache] = true
			}
		}
	}()

	jobs := make(chan *IndexJob, 2048)
	results := make(chan *IndexResult, 2048)

	var wg sync.WaitGroup

	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					CapturePanic(r, "workspace.worker")
				}
			}()

			p := parser.New(nil, ast.NewTree(nil), s.MaxParseErrors)

			for job := range jobs {
				var (
					b   []byte
					err error
				)

				if job.IsStd {
					b, err = stdlibFS.ReadFile("stdlib/" + job.Path)
				} else {
					b, err = os.ReadFile(job.Path)
				}

				if err != nil {
					results <- &IndexResult{Job: job, Uri: job.Uri, Err: err}

					continue
				}

				if job.ExistingSource != nil && bytes.Equal(job.ExistingSource, b) {
					results <- &IndexResult{Job: job, Uri: job.Uri, Unchanged: true, Source: b}

					continue
				}

				cache := jobCaches[job]
				cacheKey := parseCacheKey(b, s.MaxParseErrors)
				var tree *ast.Tree
				var errs []parser.ParseError
				var cached bool
				if job.ExistingTree == nil {
					tree, errs, cached = cache.load(cacheKey, b, s.positionEncoding)
				}
				if !cached {
					tree = job.ExistingTree
					if tree == nil {
						tree = ast.NewTree(b)
					} else {
						tree.Reset(b)
					}
					tree.SetPositionEncoding(s.positionEncoding)
					p.MaxErrors = s.MaxParseErrors
					p.Reset(b, tree)
					p.Parse()
					errs = append([]parser.ParseError(nil), p.Errors...)
					cache.save(cacheKey, tree, errs)
				}
				resolver := cache.loadSemantic(cacheKey, tree)
				if resolver == nil {
					resolver = semantic.New(tree)
					resolver.Resolve(tree.Root)
					resolver.Cleanup()
					cache.saveSemantic(cacheKey, resolver)
				}

				results <- &IndexResult{
					Job:      job,
					Uri:      job.Uri,
					Source:   b,
					Tree:     tree,
					Errors:   errs,
					Resolver: resolver,
				}
			}
		})
	}

	go func() {
		for _, job := range pendingJobs {
			jobs <- job
		}

		close(jobs)
	}()

	go func() {
		wg.Wait()

		close(results)
	}()

	completedResults := make([]*IndexResult, 0, len(pendingJobs))
	for res := range results {
		uri := res.Uri

		if s.activeURIs != nil {
			s.activeURIs[uri] = true
		}

		if res.Err != nil {
			failed++

			continue
		}

		if res.Unchanged {
			unchanged++

			if !res.Job.ModTime.IsZero() && s.Documents[uri] != nil {
				s.Documents[uri].ModTime = res.Job.ModTime
			}

			continue
		}

		if res.Job.Doc == nil {
			moduleNamesChanged = true
		}
		completedResults = append(completedResults, res)
	}

	isManifestURI := func(uri string) bool {
		return strings.HasSuffix(uri, "/fxmanifest.lua") || strings.HasSuffix(uri, "/__resource.lua")
	}
	manifestResults := make([]*IndexResult, 0)
	scriptResults := make([]*IndexResult, 0, len(completedResults))
	for _, res := range completedResults {
		if isManifestURI(res.Uri) {
			manifestResults = append(manifestResults, res)
		} else {
			scriptResults = append(scriptResults, res)
		}
	}
	sort.Slice(scriptResults, func(i, j int) bool { return scriptResults[i].Uri < scriptResults[j].Uri })
	sort.Slice(manifestResults, func(i, j int) bool {
		left, right := manifestResults[i].Uri, manifestResults[j].Uri
		leftLegacy, rightLegacy := strings.HasSuffix(left, "/__resource.lua"), strings.HasSuffix(right, "/__resource.lua")
		if filepath.Dir(left) == filepath.Dir(right) && leftLegacy != rightLegacy {
			return !leftLegacy // fxmanifest.lua is authoritative over the legacy file.
		}
		return left < right
	})
	finalizeResults := func(items []*IndexResult) {
		for _, res := range items {
			total += len(res.Source)
			indexed++
			s.finalizeDocumentUpdate(res.Uri, res.Source, res.Tree, res.Errors, res.Job.Doc, res.Resolver)
			if indexed%50 == 0 {
				s.sendProgressNotification("indexing", WorkDoneProgressReport{
					Kind:       "report",
					Message:    fmt.Sprintf("Indexed %d files...", indexed),
					Percentage: 0,
				})
			}
			if !res.Job.ModTime.IsZero() && s.Documents[res.Uri] != nil {
				s.Documents[res.Uri].ModTime = res.Job.ModTime
			}
		}
	}
	finalizeResults(manifestResults)

	// Rebuild the complete current manifest topology, including unchanged/open
	// buffers, before publishing any script result. Sorting gives collisions a
	// repeatable graph registration order; fxmanifest wins over __resource.
	manifests := make([]string, 0)
	for uri, doc := range s.Documents {
		// A failed scan cannot tell us that an unseen manifest disappeared.
		// Keep every previously indexed manifest in the rebuilt resource graph
		// until a complete scan can confirm removals.
		if doc != nil && doc.IsFiveMManifest && isManifestURI(uri) && (!workspaceScanComplete || s.activeURIs[uri] || s.OpenFiles[uri]) {
			manifests = append(manifests, uri)
		}
	}
	sort.Slice(manifests, func(i, j int) bool {
		left, right := manifests[i], manifests[j]
		leftLegacy, rightLegacy := strings.HasSuffix(left, "/__resource.lua"), strings.HasSuffix(right, "/__resource.lua")
		if filepath.Dir(left) == filepath.Dir(right) && leftLegacy != rightLegacy {
			return !leftLegacy
		}
		return left < right
	})
	registeredRoots := make(map[string]bool, len(manifests))
	for _, uri := range manifests {
		manifest := s.parseFiveMManifest(s.Documents[uri])
		if manifest == nil || registeredRoots[manifest.RootURI] {
			continue
		}
		registeredRoots[manifest.RootURI] = true
		s.registerFiveMManifestResource(manifest)
	}
	s.reconcileFiveMBindings("", oldBindings)
	for root, previous := range oldResources {
		current := s.FiveMResourceGraph.ResourceByRoot(root)
		if previous == nil || (current != nil && current.Equal(previous)) {
			continue
		}
		for dURI, affected := range s.Documents {
			if affected != nil && (dURI == root || strings.HasPrefix(dURI, root+"/")) {
				affected.FiveMEvents = affected.FiveMEvents[:0]
			}
		}
	}
	if s.GlobalIndex != nil {
		for root := range oldResources {
			if !registeredRoots[root] {
				s.GlobalIndex.UnregisterFiveMResource(ResourceURI(root))
			}
		}
	}
	finalizeResults(scriptResults)

	for uri, source := range s.ignoredOpenSources {
		if s.OpenFiles[uri] && !s.isIgnoredURI(uri) {
			delete(s.ignoredOpenSources, uri)
			delete(s.oversizedOpenFiles, uri)
			if source == nil {
				s.sendShowMessage(2, "Lugo: reopen an excluded oversized document to restore its current contents.")
			} else if s.MaxFileSize > 0 && int64(len(source)) > s.MaxFileSize {
				s.publishFileSizeDiagnostic(uri)
			} else {
				s.updateDocument(uri, source)
				moduleNamesChanged = true
			}
		}
	}

	if workspaceScanComplete {
		for uri := range s.Documents {
			if !s.activeURIs[uri] && !s.OpenFiles[uri] {
				moduleNamesChanged = true
				s.clearDocument(uri)
			}
		}
	}

	for uri, doc := range s.Documents {
		if s.OpenFiles[uri] && !s.activeURIs[uri] {
			total += len(doc.Source())

			s.updateDocument(uri, doc.Source())
		}
	}

	// Root changes can alter classification and module names for unchanged documents.
	var reclassified []*Document
	needsRebuild := make(map[*Document]bool)
	for uri, doc := range s.Documents {
		isLibrary := s.checkIsLibrary(uri, doc.LowerPath)
		isWorkspace := s.checkIsWorkspace(uri, doc.LowerPath)
		moduleName := s.computeModuleName(uri, doc.Path, doc.LowerPath)
		if doc.IsLibrary == isLibrary && doc.IsWorkspace == isWorkspace && doc.ModuleName == moduleName {
			continue
		}
		if doc.IsWorkspace && !isWorkspace && !s.OpenFiles[uri] && !s.IsCI {
			WriteMessage(s.Writer, OutgoingNotification{RPC: "2.0", Method: "textDocument/publishDiagnostics", Params: PublishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}}})
		}
		moduleNamesChanged = moduleNamesChanged || doc.ModuleName != moduleName
		doc.IsLibrary, doc.IsWorkspace, doc.ModuleName = isLibrary, isWorkspace, moduleName
		doc.FiveMProfile = FiveMExecutionProfile{}
		doc.FiveMProfileCached = false
		reclassified = append(reclassified, doc)
		needsRebuild[doc] = true
	}
	if moduleNamesChanged {
		for _, doc := range s.Documents {
			if !needsRebuild[doc] && doc.Resolver != nil && len(doc.Resolver.PendingFields) > 0 {
				reclassified = append(reclassified, doc)
			}
		}
	}
	for _, doc := range s.Documents {
		if doc.IsFiveMManifest {
			s.registerFiveMManifestResource(s.parseFiveMManifest(doc))
		}
	}
	for _, doc := range reclassified {
		if doc.Tree == nil {
			continue
		}
		if doc.Tree.Source == nil {
			source, err := os.ReadFile(doc.Path)
			if err != nil {
				s.Log.Warnf("Cannot reload reclassified document %s: %v\n", doc.Path, err)
				continue
			}
			doc.Tree.Reset(source)
			p := s.sharedParser
			p.MaxErrors = s.MaxParseErrors
			p.Reset(source, doc.Tree)
			p.Parse()
			s.finalizeDocumentUpdate(doc.URI, source, doc.Tree, p.Errors, doc)
		} else {
			s.finalizeDocumentUpdate(doc.URI, doc.Source(), doc.Tree, doc.Errors, doc)
		}
	}

	s.flushMetatableInvalidationBatch()
	s.activeURIs = nil
	s.IsIndexing = false

	took := time.Since(start)

	s.sendProgressNotification("indexing", WorkDoneProgressReport{
		Kind: "report", Message: "Analyzing workspace diagnostics...",
	})

	s.Log.Printf("Re-indexed workspace in %s (indexed=%d, unchanged=%d, failed=%d)\n", took, indexed, unchanged, failed)

	s.publishWorkspaceDiagnostics(func(done, total int) {
		s.sendProgressNotification("indexing", WorkDoneProgressReport{
			Kind: "report", Message: fmt.Sprintf("Analyzing diagnostics: %d/%d files", done, total),
		})
	})

	took = time.Since(start)
	s.sendProgressNotification("indexing", WorkDoneProgressEnd{
		Kind: "end", Message: fmt.Sprintf("Workspace ready in %s", took.Round(time.Millisecond)),
	})

	s.Log.Printf("Total time taken for %d bytes: %s\n", total, took)
}

func deduplicateIndexJobs(jobs []*IndexJob) []*IndexJob {
	if len(jobs) < 2 {
		return jobs
	}
	seen := make(map[string]struct{}, len(jobs))
	unique := jobs[:0]
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if _, exists := seen[job.Uri]; exists {
			continue
		}
		seen[job.Uri] = struct{}{}
		unique = append(unique, job)
	}
	return unique
}

// workspaceIndexBytesBucket deliberately exposes only a coarse size range.
// Workspace sizes can otherwise reveal more than indexing telemetry needs.
func workspaceIndexBytesBucket(total int) string {
	switch {
	case total == 0:
		return "0"
	case total <= 4<<10:
		return "1-4KiB"
	case total <= 64<<10:
		return "4-64KiB"
	case total <= 1<<20:
		return "64KiB-1MiB"
	case total <= 16<<20:
		return "1-16MiB"
	default:
		return "16MiB+"
	}
}

func (s *Server) indexWorkspace(rootPathOrURI string, pendingJobs *[]*IndexJob, unchanged, failed *int) bool {
	var path string

	if strings.HasPrefix(rootPathOrURI, "file://") {
		path = s.uriToPath(rootPathOrURI)
	} else {
		path = rootPathOrURI
	}

	path = strings.ReplaceAll(path, "/", string(filepath.Separator))

	stat := s.workspaceStat
	if stat == nil {
		stat = os.Stat
	}
	if _, err := stat(path); err != nil {
		if os.IsNotExist(err) {
			s.Log.Warnf("%q not found\n", path)
			return true
		}
		*failed++
		return false
	}

	if s.visitedDirs == nil {
		s.visitedDirs = make(map[string]bool, 256)
	}

	complete := true
	readDir := s.workspaceReadDir
	if readDir == nil {
		readDir = os.ReadDir
	}
	var walk func(dir string, isSymlink bool)

	walk = func(dir string, isSymlink bool) {
		realDir := dir
		if isSymlink {
			if r, err := filepath.EvalSymlinks(dir); err == nil {
				realDir = r
			}
		}

		if s.visitedDirs[realDir] {
			return
		}

		s.visitedDirs[realDir] = true

		entries, err := readDir(realDir)
		if err != nil {
			*failed++
			complete = false
			return
		}

		for _, e := range entries {
			fullPath := filepath.Join(realDir, e.Name())

			isDir := e.IsDir()
			name := e.Name()

			if s.isIgnored(fullPath, name) {
				continue
			}

			var (
				fileStat fs.FileInfo
				statErr  error
			)

			isSym := e.Type()&fs.ModeSymlink != 0
			if isSym {
				fileStat, statErr = stat(fullPath)
				if statErr == nil {
					isDir = fileStat.IsDir()
				} else {
					*failed++
					complete = false
					continue
				}
			}

			if isDir {
				walk(fullPath, isSym)
			} else if len(name) > 4 && name[len(name)-4:] == ".lua" {
				uri := s.pathToURI(fullPath)
				// Workspace folders and library paths can overlap. Mark jobs when
				// queued so the same tree is never parsed by multiple workers.
				if s.activeURIs != nil && s.activeURIs[uri] {
					continue
				}

				s.uriCache[uri] = uri

				if s.OpenFiles[uri] {
					if s.activeURIs != nil {
						s.activeURIs[uri] = true
					}

					*unchanged++

					continue
				}

				if !isSym {
					fileStat, statErr = e.Info()
				}

				if statErr != nil {
					*failed++
					complete = false
					continue
				}

				if statErr == nil {
					if fileStat.Size() > s.MaxFileSize {
						s.Log.Warnf("Skipping huge file %s (%d bytes)\n", fullPath, fileStat.Size())

						continue
					}

					if existing, ok := s.Documents[uri]; ok && existing.ModTime.Equal(fileStat.ModTime()) {
						if s.activeURIs != nil {
							s.activeURIs[uri] = true
						}

						*unchanged++

						continue
					}
				}

				var (
					existingTree   *ast.Tree
					existingSource []byte
					doc            *Document
				)

				if existing, ok := s.Documents[uri]; ok {
					existingTree = existing.Tree
					existingSource = existing.Source()
					doc = existing
				}

				var modTime time.Time

				if statErr == nil {
					modTime = fileStat.ModTime()
				}
				if s.activeURIs != nil {
					s.activeURIs[uri] = true
				}

				*pendingJobs = append(*pendingJobs, &IndexJob{
					Uri:            uri,
					Path:           fullPath,
					ModTime:        modTime,
					ExistingTree:   existingTree,
					ExistingSource: existingSource,
					Doc:            doc,
				})
			}
		}
	}

	walk(path, true)
	return complete
}

func (s *Server) contentExceedsMaxFileSize(text string) bool {
	return s.MaxFileSize > 0 && int64(len(text)) > s.MaxFileSize
}

func (s *Server) publishFileSizeDiagnostic(uri string) {
	diag := Diagnostic{
		Range:    Range{},
		Severity: SeverityError,
		Code:     "file-too-large",
		Message:  fmt.Sprintf("Document exceeds the maximum file size of %d bytes.", s.MaxFileSize),
	}

	if s.IsCI {
		s.printCIDiagnostics(uri, []Diagnostic{diag})
		return
	}

	WriteMessage(s.Writer, OutgoingNotification{
		RPC:    "2.0",
		Method: "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: []Diagnostic{diag},
		},
	})
}

func (s *Server) updateDocument(uri string, source []byte) bool {
	if s.MaxFileSize > 0 && int64(len(source)) > s.MaxFileSize {
		return false
	}

	var (
		tree *ast.Tree
		doc  *Document
	)

	if existing, exists := s.Documents[uri]; exists {
		if existing.Tree.Source != nil && bytes.Equal(existing.Tree.Source, source) {
			return false
		}

		doc = existing
		tree = existing.Tree

		tree.Reset(source)
	} else {
		tree = ast.NewTree(source)
	}
	tree.SetPositionEncoding(s.positionEncoding)

	p := s.sharedParser

	p.MaxErrors = s.MaxParseErrors

	p.Reset(source, tree)

	p.Parse()

	errs := make([]parser.ParseError, len(p.Errors))

	copy(errs, p.Errors)

	return s.finalizeDocumentUpdate(uri, source, tree, errs, doc)
}

func (s *Server) finalizeDocumentUpdate(uri string, source []byte, tree *ast.Tree, parseErrors []parser.ParseError, doc *Document, resolved ...*semantic.Resolver) bool {
	var needsWorkspaceRepublish bool
	var oldPublicFacts uint64
	hadDocument := doc != nil
	if doc != nil {
		oldPublicFacts = documentPublicFacts(doc)
	}
	tree.SetPositionEncoding(s.positionEncoding)
	s.forgetMetatableTypeDependencyConsumer(uri)

	if doc != nil {
		// Canonical source is owned by the Tree; ensure Tree reflects the latest source.
		// Do not assign to doc.Source here.
		s.removeDocumentGlobals(uri)
		doc.clearEvictedMetatableNodeText()

		doc.ExportedGlobalDefs = doc.ExportedGlobalDefs[:0]

		doc.Tree = tree
	} else {
		doc = &Document{
			Server: s,
			URI:    uri,
			Tree:   tree,
		}
		if len(resolved) == 0 || resolved[0] == nil {
			doc.Resolver = semantic.New(tree)
		}

		doc.Path = s.uriToPath(uri)
		doc.LowerPath = strings.ToLower(doc.Path)
		doc.Dir = filepath.Dir(doc.Path)
		doc.IsLibrary = s.checkIsLibrary(uri, doc.LowerPath)
		doc.IsWorkspace = s.checkIsWorkspace(uri, doc.LowerPath)
		doc.ModuleName = s.computeModuleName(uri, doc.Path, doc.LowerPath)
		doc.IsFiveMManifest = strings.HasSuffix(uri, "/fxmanifest.lua") || strings.HasSuffix(uri, "/__resource.lua")

		s.Documents[uri] = doc
	}

	rootID := tree.Root

	doc.IsMeta = false
	doc.FiveMProfile = FiveMExecutionProfile{}
	doc.FiveMProfileCached = false
	doc.FiveMEvents = doc.FiveMEvents[:0]

	if s.GlobalIndex != nil {
		s.GlobalIndex.EnsureResource(ResourceURI(uri))
		s.GlobalIndex.SetSource(ResourceURI(uri), source, tree)
	}

	for _, c := range tree.Comments {
		if bytes.Contains(tree.Source[c.Start:c.End], []byte("@meta")) {
			doc.IsMeta = true

			break
		}
	}

	if len(parseErrors) > 0 {
		if cap(doc.Errors) >= len(parseErrors) {
			doc.Errors = doc.Errors[:len(parseErrors)]
		} else {
			doc.Errors = make([]parser.ParseError, len(parseErrors))
		}

		copy(doc.Errors, parseErrors)
	} else {
		doc.Errors = doc.Errors[:0]
	}

	res := doc.Resolver
	if len(resolved) > 0 && resolved[0] != nil {
		res = resolved[0]
		doc.Resolver = res
	} else {
		res.Tree = tree
		res.Reset()
		res.Resolve(rootID)
		res.Cleanup()
	}
	doc.invalidateMetatableAttachmentIndex()

	doc.parseDiagnosticPragmas()

	doc.FiveMLuaExports = doc.FiveMLuaExports[:0]

	for i := 1; i < len(tree.Nodes); i++ {
		node := tree.Nodes[i]
		if node.Kind == ast.KindCallExpr && int(node.Left) < len(tree.Nodes) {
			leftNode := tree.Nodes[node.Left]
			if leftNode.Kind == ast.KindIdent && doc.referenceAt(node.Left) == ast.InvalidNode {
				if bytes.Equal(doc.Source()[leftNode.Start:leftNode.End], []byte("exports")) {
					if node.Count >= 2 && node.Extra+1 < uint32(len(tree.ExtraList)) {
						arg1ID := tree.ExtraList[node.Extra]
						arg2ID := tree.ExtraList[node.Extra+1]

						if int(arg1ID) < len(tree.Nodes) && tree.Nodes[arg1ID].Kind == ast.KindString {
							exportName := unquoteLuaString(string(doc.Source()[tree.Nodes[arg1ID].Start:tree.Nodes[arg1ID].End]))

							doc.FiveMLuaExports = append(doc.FiveMLuaExports, FiveMLuaExport{
								Name:   exportName,
								NodeID: arg2ID,
							})
						}
					}
				}
			}
		}
	}

	doc.FiveMEvents = doc.FiveMEvents[:0]

	for i := 1; i < len(tree.Nodes); i++ {
		node := tree.Nodes[i]
		if node.Kind == ast.KindCallExpr && int(node.Left) < len(tree.Nodes) {
			leftNode := tree.Nodes[node.Left]
			if leftNode.Kind == ast.KindMemberExpr && leftNode.Right != ast.InvalidNode && int(leftNode.Right) < len(tree.Nodes) && tree.Nodes[leftNode.Right].Kind == ast.KindIdent && node.Count >= 2 && node.Extra+1 < uint32(len(tree.ExtraList)) {
				var callbackKind FiveMEventKind
				switch string(doc.nodeSource(leftNode.Right)) {
				case "RegisterServerCallback":
					callbackKind = FiveMFrameworkRegisterServerCallback
				case "TriggerServerCallback":
					callbackKind = FiveMFrameworkTriggerServerCallback
				}
				if callbackKind != 0 {
					nameID := tree.ExtraList[node.Extra]
					if name, ok := doc.fiveMStaticEventName(nameID, 0); ok {
						doc.FiveMEvents = append(doc.FiveMEvents, FiveMEventInfo{Name: name, Kind: callbackKind, NodeID: nameID, HandlerID: tree.ExtraList[node.Extra+1]})
					}
				}
			}
			if leftNode.Kind == ast.KindIdent && doc.referenceAt(node.Left) == ast.InvalidNode {
				src := doc.Source()
				ident := src[leftNode.Start:leftNode.End]

				var kind FiveMEventKind
				var extraPayloadArgs uint8
				matched := true

				switch {
				case bytes.Equal(ident, []byte("AddEventHandler")):
					kind = FiveMEventAddHandler
				case bytes.Equal(ident, []byte("RegisterNetEvent")), bytes.Equal(ident, []byte("RegisterServerEvent")):
					kind = FiveMEventRegisterNet
				case bytes.Equal(ident, []byte("TriggerEvent")):
					kind = FiveMEventTriggerLocal
				case bytes.Equal(ident, []byte("TriggerServerEvent")):
					kind = FiveMEventTriggerServer
				case bytes.Equal(ident, []byte("TriggerLatentServerEvent")):
					kind, extraPayloadArgs = FiveMEventTriggerServer, 1
				case bytes.Equal(ident, []byte("TriggerClientEvent")):
					kind = FiveMEventTriggerClient
				case bytes.Equal(ident, []byte("TriggerLatentClientEvent")):
					kind, extraPayloadArgs = FiveMEventTriggerClient, 1
				default:
					matched = false
				}

				if matched && node.Count >= 1 && node.Extra < uint32(len(tree.ExtraList)) {
					arg1ID := tree.ExtraList[node.Extra]
					if eventName, ok := doc.fiveMStaticEventName(arg1ID, 0); ok {
						info := FiveMEventInfo{
							Name:             eventName,
							Kind:             kind,
							NodeID:           arg1ID,
							ExtraPayloadArgs: extraPayloadArgs,
						}

						// For AddEventHandler and RegisterNetEvent, record handler NodeID (second arg)
						if (kind == FiveMEventAddHandler || kind == FiveMEventRegisterNet) && node.Count >= 2 && node.Extra+1 < uint32(len(tree.ExtraList)) {
							info.HandlerID = tree.ExtraList[node.Extra+1]
						}

						doc.FiveMEvents = append(doc.FiveMEvents, info)
					}
				}
			}
		}
	}

	doc.ExportedNode = ast.InvalidNode

	if rootID != ast.InvalidNode && int(rootID) < len(tree.Nodes) {
		root := tree.Nodes[rootID]
		if root.Kind == ast.KindFile && root.Left != ast.InvalidNode {
			block := tree.Nodes[root.Left]

			for i := uint32(0); i < block.Count; i++ {
				if block.Extra+uint32(i) >= uint32(len(tree.ExtraList)) {
					continue
				}

				stmtID := tree.ExtraList[block.Extra+uint32(i)]
				if int(stmtID) < len(tree.Nodes) {
					stmt := tree.Nodes[stmtID]
					if stmt.Kind == ast.KindReturn && stmt.Left != ast.InvalidNode {
						exprList := tree.Nodes[stmt.Left]
						if exprList.Count > 0 && exprList.Extra < uint32(len(tree.ExtraList)) {
							doc.ExportedNode = tree.ExtraList[exprList.Extra]
						}
					}
				}
			}
		}
	}

	fieldDefsByLocal := make(map[ast.NodeID][]int, len(res.FieldDefs))

	for i, fd := range res.FieldDefs {
		if fd.ReceiverDef != ast.InvalidNode {
			fieldDefsByLocal[fd.ReceiverDef] = append(fieldDefsByLocal[fd.ReceiverDef], i)
		}
	}

	for i := 0; i < len(tree.Comments); {
		startComment := tree.Comments[i]
		lastComment := startComment

		var nextIdx int

		for j := i + 1; j < len(tree.Comments); j++ {
			nextC := tree.Comments[j]

			gap := tree.Source[lastComment.End:nextC.Start]
			if bytes.Count(gap, []byte{'\n'}) <= 1 {
				lastComment = nextC
			} else {
				nextIdx = j

				break
			}
		}

		if nextIdx == 0 {
			nextIdx = len(tree.Comments)
		}

		fullBlock := tree.Source[startComment.Start:lastComment.End]

		if !bytes.Contains(fullBlock, []byte("@class")) && !bytes.Contains(fullBlock, []byte("@alias")) && !bytes.Contains(fullBlock, []byte("@export")) {
			i = nextIdx

			continue
		}

		s.sharedCommentBuf = s.sharedCommentBuf[:0]
		cleaned := cleanLuaCommentBytes(s.sharedCommentBuf, fullBlock)

		luadoc := parseLuaDoc(cleaned, s.FeatureFormatAlerts)

		var virtualNodeID = ast.InvalidNode

		if luadoc.Export != "" {
			virtualNodeID = ast.NodeID(len(tree.Nodes))

			tree.Nodes = append(tree.Nodes, ast.Node{
				Kind:   ast.KindFunctionExpr,
				Start:  lastComment.End,
				End:    lastComment.End,
				Parent: ast.InvalidNode,
			})

			doc.FiveMLuaExports = append(doc.FiveMLuaExports, FiveMLuaExport{
				Name:   luadoc.Export,
				NodeID: virtualNodeID,
			})
		}

		var typeName string

		if luadoc.Class != nil && luadoc.Class.Name != "" {
			typeName = luadoc.Class.Name
		} else if luadoc.Alias != nil && luadoc.Alias.Name != "" {
			typeName = luadoc.Alias.Name
		}

		if typeName != "" {
			nameBytes := []byte(typeName)

			var (
				recHash  uint64
				propHash uint64
			)

			lastDot := bytes.LastIndexByte(nameBytes, '.')
			if lastDot != -1 {
				recHash = ast.HashBytes(nameBytes[:lastDot])
				propHash = ast.HashBytes(nameBytes[lastDot+1:])
			} else {
				recHash = 0
				propHash = ast.HashBytes(nameBytes)
			}

			var parentName string

			if luadoc.Class != nil {
				parentName = luadoc.Class.Parent
			}

			if virtualNodeID == ast.InvalidNode {
				virtualNodeID = ast.NodeID(len(tree.Nodes))

				tree.Nodes = append(tree.Nodes, ast.Node{
					Kind:   ast.KindTableExpr,
					Start:  lastComment.End,
					End:    lastComment.End,
					Parent: ast.InvalidNode,
				})
			}

			s.setGlobalSymbol(GlobalKey{ReceiverHash: recHash, PropHash: propHash}, uri, virtualNodeID, typeName, parentName, true, luadoc.IsDeprecated, luadoc.DeprecatedMsg)
			if luadoc.Class != nil && s.GlobalIndex != nil {
				resource, scope := ResourceURI(uri), GlobalIndexScopeShared
				if indexedDoc := s.Documents[uri]; indexedDoc != nil {
					resource, scope = s.globalIndexContext(indexedDoc)
					if resource == "" {
						resource = ResourceURI(uri)
					}
				}
				var entry *SymbolEntry
				for _, candidate := range s.GlobalIndex.LookupCandidatesByScope(resource, scope, SymbolName(typeName)) {
					if candidate != nil && candidate.URI == uri && candidate.NodeID == virtualNodeID {
						entry = candidate
						break
					}
				}
				if entry != nil {
					indexedEntry := *entry
					if len(luadoc.Class.GenericParams) > 0 || luaDocClassHasGenericParent(luadoc.Class.Parents) {
						data := LuaDocData{}
						if entry.LuaDoc != nil {
							data = *entry.LuaDoc
						}
						data.GenericClass = &LuaDocGenericClassData{
							Params:  append([]LuaDocGeneric(nil), luadoc.Class.GenericParams...),
							Parents: append([]string(nil), luadoc.Class.Parents...),
						}
						indexedEntry.LuaDoc = &data
					}
					indexedEntry.Parents = make([]string, 0, len(luadoc.Class.Parents))
					for _, parentType := range luadoc.Class.Parents {
						indexedEntry.Parents = append(indexedEntry.Parents, luaDocGenericClassName(parentType))
					}
					indexedEntry.Parent = ""
					if len(indexedEntry.Parents) > 0 {
						indexedEntry.Parent = indexedEntry.Parents[0]
					}
					s.setGlobalIndexSymbol(resource, scope, SymbolName(typeName), &indexedEntry)
				}
			}
			if (len(luadoc.Operators) > 0 || len(luadoc.Overloads) > 0) && s.GlobalIndex != nil {
				resource, scope := ResourceURI(uri), GlobalIndexScopeShared
				if indexedDoc := s.Documents[uri]; indexedDoc != nil {
					resource, scope = s.globalIndexContext(indexedDoc)
				}
				var entry *SymbolEntry
				for _, candidate := range s.GlobalIndex.LookupCandidatesByScope(resource, scope, SymbolName(typeName)) {
					if candidate != nil && candidate.URI == uri && candidate.NodeID == virtualNodeID {
						entry = candidate
						break
					}
				}
				if entry != nil {
					indexedEntry := *entry
					data := LuaDocData{}
					if entry.LuaDoc != nil {
						data = *entry.LuaDoc
					}
					data.Operators = append([]LuaDocOperator(nil), luadoc.Operators...)
					data.Overloads = append([]string(nil), luadoc.Overloads...)
					indexedEntry.LuaDoc = &data
					s.setGlobalIndexSymbol(resource, scope, SymbolName(typeName), &indexedEntry)
				}
			}

			if len(luadoc.Fields) > 0 {
				classHash := ast.HashBytes(nameBytes)

				for _, field := range luadoc.Fields {
					fieldHash := ast.HashBytes([]byte(field.Name))

					var sb strings.Builder

					sb.Grow(len(nameBytes) + 1 + len(field.Name))
					sb.Write(nameBytes)
					sb.WriteByte('.')
					sb.WriteString(field.Name)

					fieldVirtualNodeID := ast.NodeID(len(tree.Nodes))

					var kind = ast.KindIdent
					if strings.Contains(field.Type, "fun") || strings.Contains(field.Type, "function") {
						kind = ast.KindFunctionExpr
					}

					tree.Nodes = append(tree.Nodes, ast.Node{
						Kind:   kind,
						Start:  lastComment.End,
						End:    lastComment.End,
						Parent: ast.InvalidNode,
					})

					s.setGlobalSymbol(GlobalKey{ReceiverHash: classHash, PropHash: fieldHash}, uri, fieldVirtualNodeID, sb.String(), "", true, luadoc.IsDeprecated, luadoc.DeprecatedMsg)

					// Keep the parsed LuaDoc field type on the indexed entry so
					// cross-document consumers can resolve both the field and type.
					if s.GlobalIndex != nil {
						resource := ResourceURI(uri)
						scope := GlobalIndexScopeShared
						if indexedDoc := s.Documents[uri]; indexedDoc != nil {
							resource, scope = s.globalIndexContext(indexedDoc)
							if resource == "" {
								resource = ResourceURI(uri)
							}
						}
						var entry *SymbolEntry
						for _, candidate := range s.GlobalIndex.LookupCandidatesByScope(resource, scope, SymbolName(sb.String())) {
							if candidate != nil && candidate.URI == uri && candidate.NodeID == fieldVirtualNodeID {
								entry = candidate
								break
							}
						}
						if entry != nil {
							indexedEntry := *entry
							indexedEntry.LuaDoc = &LuaDocData{TypeString: field.Type}
							parsedType := indexedEntry.LuaDoc.ParsedType()
							indexedEntry.Type = Type{Primitive: parsedType.Basics}
							if parsedType.IsCallable() {
								indexedEntry.Type.Structural = &StructuralType{Function: &FunctionType{}}
							}
							s.setGlobalIndexSymbol(resource, scope, SymbolName(sb.String()), &indexedEntry)
						}
					}
				}
			}
		}

		i = nextIdx
	}

	if missing := len(tree.Nodes) - len(res.References); missing > 0 {
		for i := 0; i < missing; i++ {
			res.References = append(res.References, ast.InvalidNode)
		}
	}

	for _, defID := range res.GlobalDefs {
		node := tree.Nodes[defID]
		if node.Start == node.End {
			continue
		}

		identBytes := tree.Source[node.Start:node.End]
		hash := ast.HashBytes(identBytes)

		isRoot := isRootLevel(tree, defID)
		isDep, depMsg := doc.HasDeprecatedTag(defID)

		s.setGlobalSymbol(GlobalKey{ReceiverHash: 0, PropHash: hash}, uri, defID, string(identBytes), "", isRoot, isDep, depMsg)

		for name := range doc.ExtractLuaDocFields(defID) {
			fieldHash := ast.HashBytes(name)

			var sb strings.Builder

			sb.Grow(len(identBytes) + 1 + len(name))
			sb.Write(identBytes)
			sb.WriteByte('.')
			sb.Write(name)

			s.setGlobalSymbol(GlobalKey{ReceiverHash: hash, PropHash: fieldHash}, uri, defID, sb.String(), "", isRoot, isDep, depMsg)
		}

		// Module Aliasing
		valID := doc.getAssignedValue(defID)

		if valID != ast.InvalidNode {
			valNode := tree.Nodes[valID]

			if valNode.Kind == ast.KindIdent {
				localDefID := doc.referenceAt(valID)

				if localDefID != ast.InvalidNode {
					localName := doc.Source()[doc.Tree.Nodes[localDefID].Start:doc.Tree.Nodes[localDefID].End]
					globalBytes := tree.Source[node.Start:node.End]

					for _, fdIdx := range fieldDefsByLocal[localDefID] {
						fd := res.FieldDefs[fdIdx]

						fdIsRoot := isRootLevel(tree, fd.NodeID)
						fdIsDep, fdDepMsg := doc.HasDeprecatedTag(fd.NodeID)

						if bytes.Equal(fd.ReceiverName, localName) {
							propBytes := doc.Source()[doc.Tree.Nodes[fd.NodeID].Start:doc.Tree.Nodes[fd.NodeID].End]

							var sb strings.Builder

							sb.Grow(len(identBytes) + 1 + len(propBytes))
							sb.Write(identBytes)
							sb.WriteByte('.')
							sb.Write(propBytes)

							s.setGlobalSymbol(GlobalKey{ReceiverHash: hash, PropHash: fd.PropHash}, uri, fd.NodeID, sb.String(), "", fdIsRoot, fdIsDep, fdDepMsg)
						} else if len(fd.ReceiverName) > len(localName) && bytes.HasPrefix(fd.ReceiverName, localName) && fd.ReceiverName[len(localName)] == '.' {
							suffix := fd.ReceiverName[len(localName)+1:]

							newRecHash := ast.HashBytesConcat(globalBytes, []byte{'.'}, suffix)

							propBytes := doc.Source()[doc.Tree.Nodes[fd.NodeID].Start:doc.Tree.Nodes[fd.NodeID].End]

							var sb strings.Builder

							sb.Grow(len(identBytes) + 2 + len(suffix) + len(propBytes))
							sb.Write(identBytes)
							sb.WriteByte('.')
							sb.Write(suffix)
							sb.WriteByte('.')
							sb.Write(propBytes)

							s.setGlobalSymbol(GlobalKey{ReceiverHash: newRecHash, PropHash: fd.PropHash}, uri, fd.NodeID, sb.String(), "", fdIsRoot, fdIsDep, fdDepMsg)
						}
					}
				}
			}
		}
	}

	// Index global table fields
	for _, fd := range res.FieldDefs {
		var (
			globalRecName []byte
			globalRecHash uint64
		)

		if fd.ReceiverDef == ast.InvalidNode {
			globalRecName = fd.ReceiverName
			globalRecHash = fd.ReceiverHash
		} else {
			valID := doc.getAssignedValue(fd.ReceiverDef)
			if valID != ast.InvalidNode {
				globalRecName = s.getGlobalPath(doc, valID, 0)
				if globalRecName != nil {
					globalRecHash = ast.HashBytes(globalRecName)
				}
			}

			// Fallback: when getGlobalPath cannot resolve the path (e.g. the receiver's
			// assigned value is a table literal or call expression), use the pre-computed
			// receiver name and hash from the resolver's receiver context.
			if globalRecName == nil && len(fd.ReceiverName) > 0 {
				globalRecName = fd.ReceiverName
				globalRecHash = fd.ReceiverHash
			}
		}

		if globalRecName != nil {
			if bytes.Equal(globalRecName, []byte("self")) {
				continue
			}

			propBytes := doc.Source()[doc.Tree.Nodes[fd.NodeID].Start:doc.Tree.Nodes[fd.NodeID].End]

			sep := byte('.')

			if doc.Tree.Nodes[doc.Tree.Nodes[fd.NodeID].Parent].Kind == ast.KindMethodName {
				sep = ':'
			}

			var sb strings.Builder

			sb.Grow(len(globalRecName) + 1 + len(propBytes))
			sb.Write(globalRecName)
			sb.WriteByte(sep)
			sb.Write(propBytes)

			isRoot := isRootLevel(tree, fd.NodeID)
			isDep, depMsg := doc.HasDeprecatedTag(fd.NodeID)

			s.setGlobalSymbol(GlobalKey{ReceiverHash: globalRecHash, PropHash: fd.PropHash}, uri, fd.NodeID, sb.String(), "", isRoot, isDep, depMsg)

			// Register class-to-table alias: when a table field has a @type annotation
			// pointing to a class (e.g. ---@type Account on Core.Account = {}),
			// map the class hash to the FULL table path hash so that completion on
			// typed variables can find colon methods indexed under that path.
			if luadoc := doc.GetLuaDoc(fd.NodeID); luadoc != nil && luadoc.Type != nil && luadoc.Type.Type != "" {
				typeHash := ast.HashBytes([]byte(luadoc.Type.Type))

				// Use the full table path (rec.prop) as the alias target, not just rec.
				// e.g. for Core.Account = {}, the full path is "Core.Account"
				tablePathHash := ast.HashBytes([]byte(sb.String()))

				s.setTableAlias(uri, typeHash, tablePathHash)
			}
		}
	}

	if doc.ExportedNode != ast.InvalidNode {
		modName := doc.ModuleName
		if modName == "" {
			modName = "module"
		}

		modHash := ast.HashBytesConcat([]byte("module:"), nil, []byte(uri))

		exportNode := doc.Tree.Nodes[doc.ExportedNode]
		if exportNode.Kind == ast.KindIdent {
			exportDef := doc.referenceAt(doc.ExportedNode)
			if exportDef != ast.InvalidNode {
				exportDefNode := doc.Tree.Nodes[exportDef]
				exportHash := ast.HashBytes(doc.Source()[exportDefNode.Start:exportDefNode.End])

				for _, fd := range doc.Resolver.FieldDefs {
					if fd.ReceiverDef == exportDef && fd.ReceiverHash == exportHash {
						propName := doc.Source()[doc.Tree.Nodes[fd.NodeID].Start:doc.Tree.Nodes[fd.NodeID].End]

						isRoot := isRootLevel(doc.Tree, fd.NodeID)
						isDep, depMsg := doc.HasDeprecatedTag(fd.NodeID)

						s.setGlobalSymbol(GlobalKey{ReceiverHash: modHash, PropHash: fd.PropHash}, uri, fd.NodeID, modName+"."+string(propName), "", isRoot, isDep, depMsg)
					}
				}
			}
		} else if exportNode.Kind == ast.KindTableExpr {
			for i := uint32(0); i < exportNode.Count; i++ {
				if exportNode.Extra+uint32(i) >= uint32(len(doc.Tree.ExtraList)) {
					continue
				}

				fieldID := doc.Tree.ExtraList[exportNode.Extra+uint32(i)]
				if int(fieldID) < len(doc.Tree.Nodes) {
					field := doc.Tree.Nodes[fieldID]
					if field.Kind == ast.KindRecordField {
						key := doc.Tree.Nodes[field.Left]
						if key.Kind == ast.KindIdent {
							propHash := ast.HashBytes(doc.Source()[key.Start:key.End])
							propName := doc.Source()[key.Start:key.End]

							isRoot := isRootLevel(doc.Tree, field.Left)
							isDep, depMsg := doc.HasDeprecatedTag(field.Left)

							s.setGlobalSymbol(GlobalKey{ReceiverHash: modHash, PropHash: propHash}, uri, field.Left, modName+"."+string(propName), "", isRoot, isDep, depMsg)
						}
					}
				}
			}
		}
	}

	for i, pf := range res.PendingFields {
		var reqModName string

		if pf.ReceiverDef != ast.InvalidNode {
			valID := doc.getAssignedValue(pf.ReceiverDef)
			reqModName = s.getRequireModName(doc, valID)
		} else {
			parentID := doc.Tree.Nodes[pf.PropNodeID].Parent
			if parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
				parentNode := doc.Tree.Nodes[parentID]
				if parentNode.Kind == ast.KindMemberExpr || parentNode.Kind == ast.KindMethodCall {
					reqModName = s.getRequireModName(doc, parentNode.Left)
				}
			}
		}

		if reqModName != "" {
			targetDoc := s.resolveModule(uri, reqModName)
			if targetDoc != nil {
				modHash := ast.HashBytesConcat([]byte("module:"), nil, []byte(targetDoc.URI))
				res.PendingFields[i].ReceiverHash = modHash
			}
		}
	}

	doc.TypeCache.Reset(len(tree.Nodes))

	if cap(doc.Inferring) >= len(tree.Nodes) {
		doc.Inferring = doc.Inferring[:len(tree.Nodes)]

		clear(doc.Inferring)
	} else {
		doc.Inferring = make([]bool, len(tree.Nodes))
	}

	if cap(doc.LuaDocCache) >= len(tree.Nodes) {
		doc.LuaDocCache = doc.LuaDocCache[:len(tree.Nodes)]

		clear(doc.LuaDocCache)
	} else {
		doc.LuaDocCache = make([]*LuaDoc, len(tree.Nodes))
	}

	if cap(doc.ActualReads) >= len(tree.Nodes) {
		doc.ActualReads = doc.ActualReads[:len(tree.Nodes)]

		clear(doc.ActualReads)
	} else {
		doc.ActualReads = make([]uint16, len(tree.Nodes))
	}

	if cap(doc.MutatedLocals) >= len(tree.Nodes) {
		doc.MutatedLocals = doc.MutatedLocals[:len(tree.Nodes)]

		clear(doc.MutatedLocals)
	} else {
		doc.MutatedLocals = make([]bool, len(tree.Nodes))
	}

	for refID, defID := range doc.Resolver.References {
		if defID != ast.InvalidNode && ast.NodeID(refID) != defID {
			if s.isActualRead(doc, ast.NodeID(refID)) {
				doc.ActualReads[defID]++
			}
		}
	}

	for _, reassignment := range doc.Resolver.Reassignments {
		if reassignment.DefID != ast.InvalidNode && int(reassignment.DefID) < len(doc.MutatedLocals) {
			doc.MutatedLocals[reassignment.DefID] = true
		}
	}

	if doc.IsFiveMManifest {
		res := s.parseFiveMManifest(doc)
		oldRes := s.FiveMResourceGraph.ResourceByRoot(res.RootURI)
		oldBindings := s.snapshotFiveMBindingsUnder(res.RootURI)
		var activeRes *FiveMResource
		if !s.IsIndexing {
			activeRes = s.registerFiveMManifestResource(res)
		}

		if oldRes == nil || (activeRes != nil && !activeRes.Equal(oldRes)) {
			needsWorkspaceRepublish = true
		}
		if oldRes != nil && activeRes != nil && !activeRes.Equal(oldRes) {
			for dURI, affected := range s.Documents {
				if affected != nil && (dURI == res.RootURI || strings.HasPrefix(dURI, res.RootURI+"/")) {
					affected.FiveMEvents = affected.FiveMEvents[:0]
				}
			}
		}
		// Manifest registration changes the placement used by globalIndexContext.
		// Re-publish only documents whose resource/scope changed, including
		// unchanged scripts that were finalized before this manifest existed.
		if !s.IsIndexing {
			bindingsChanged := s.reconcileFiveMBindings(res.RootURI, oldBindings)
			needsWorkspaceRepublish = bindingsChanged || needsWorkspaceRepublish
			if bindingsChanged {
				for dURI, affected := range s.Documents {
					if affected != nil && (dURI == res.RootURI || strings.HasPrefix(dURI, res.RootURI+"/")) {
						affected.FiveMEvents = affected.FiveMEvents[:0]
					}
				}
			}
		}
	}
	if hadDocument && oldPublicFacts != documentPublicFacts(doc) {
		needsWorkspaceRepublish = true
	}

	s.Documents[uri] = doc
	s.invalidateMetatableIndexes(uri)

	return needsWorkspaceRepublish
}

type fiveMBindingPlacement struct {
	resource ResourceURI
	scope    GlobalIndexScope
}

func (s *Server) snapshotFiveMBindingsUnder(root string) map[string]fiveMBindingPlacement {
	if s == nil {
		return nil
	}
	placements := make(map[string]fiveMBindingPlacement)
	for uri, doc := range s.Documents {
		if doc == nil || (root != "" && uri != root && !strings.HasPrefix(uri, root+"/")) {
			continue
		}
		resource, scope := s.globalIndexContext(doc)
		placements[uri] = fiveMBindingPlacement{resource: resource, scope: scope}
	}
	return placements
}

func (s *Server) reconcileFiveMBindings(root string, previous map[string]fiveMBindingPlacement) bool {
	if s == nil {
		return false
	}
	var changed bool
	for uri, doc := range s.Documents {
		if doc == nil || doc.IsFiveMManifest || (root != "" && uri != root && !strings.HasPrefix(uri, root+"/")) {
			continue
		}
		old, hadOld := previous[uri]
		doc.FiveMProfile = FiveMExecutionProfile{}
		doc.FiveMProfileCached = false
		currentResource, currentScope := s.globalIndexContext(doc)
		if hadOld && old.resource == currentResource && old.scope == currentScope {
			continue
		}
		// finalizeDocumentUpdate removes symbols using the old document's source
		// URI, then republishes them under the now-authoritative placement. Reuse
		// the parsed tree and parse errors; no file read or parse is needed.
		s.finalizeDocumentUpdate(uri, doc.Source(), doc.Tree, doc.Errors, doc)
		changed = true
	}
	return changed
}

// documentPublicFacts captures names that can affect cross-document diagnostics.
// Node IDs are intentionally excluded so edits to unrelated lines stay local.
func documentPublicFacts(doc *Document) uint64 {
	if doc == nil {
		return 0
	}
	const offset64 = uint64(14695981039346656037)
	const prime64 = uint64(1099511628211)
	h := offset64
	write := func(v uint64) {
		h ^= v
		h *= prime64
	}
	writeString := func(v string) {
		for i := 0; i < len(v); i++ {
			write(uint64(v[i]))
		}
		write(0xff)
	}
	for _, symbol := range doc.ExportedGlobalDefs {
		write(symbol.Key.ReceiverHash)
		write(symbol.Key.PropHash)
		if symbol.IsRoot {
			write(1)
		} else {
			write(0)
		}
		if symbol.IsDeprecated {
			write(1)
		} else {
			write(0)
		}
		writeString(symbol.DeprecatedMsg)
	}
	for _, exp := range doc.FiveMLuaExports {
		writeString(exp.Name)
	}
	for _, event := range doc.FiveMEvents {
		write(uint64(event.Kind))
		write(uint64(event.ExtraPayloadArgs))
		writeString(event.Name)
	}
	return h
}

func (s *Server) indexEmbeddedStdlib(pendingJobs *[]*IndexJob, unchanged *int) {
	entries, err := stdlibFS.ReadDir("stdlib")
	if err != nil {
		s.Log.Warnf("Failed to read embedded stdlib: %v\n", err)

		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}

		name := entry.Name()
		uri := embeddedStdlibURIPrefix + name

		if s.OpenFiles[uri] {
			if s.activeURIs != nil {
				s.activeURIs[uri] = true
			}

			*unchanged++

			continue
		}

		var (
			existingTree   *ast.Tree
			existingSource []byte
			doc            *Document
		)

		if existing, ok := s.Documents[uri]; ok {
			existingTree = existing.Tree
			existingSource = existing.Source()
			doc = existing
		}

		*pendingJobs = append(*pendingJobs, &IndexJob{
			Uri:            uri,
			Path:           name,
			IsStd:          true,
			ExistingTree:   existingTree,
			ExistingSource: existingSource,
			Doc:            doc,
		})
	}
}

func (s *Server) clearDocument(uri string) {
	resource := ResourceURI(uri)

	if doc, ok := s.Documents[uri]; ok {
		resource, _ = s.globalIndexContext(doc)
		if resource == "" {
			resource = ResourceURI(uri)
		}
		s.removeDocumentGlobals(uri)
	}

	// A resource scope may be shared by several documents (and by graph
	// dependencies). Prune only the document source; GlobalIndex keeps shared
	// metadata until its last symbol/reference disappears.
	if s.GlobalIndex != nil && !s.isGlobalIndexResourceReferencedByDocument(resource, uri) {
		s.GlobalIndex.PruneResource(ResourceURI(uri))
	} else if s.GlobalIndex != nil {
		s.GlobalIndex.EvictSource(ResourceURI(uri))
	}

	delete(s.Documents, uri)
	s.invalidateMetatableIndexes(uri)
	if batch := s.metatableInvalidationBatch; batch != nil {
		// Invalidate consumers before forgetting the removed provider's edges.
		// Keep batching subsequent refresh work after this prune boundary.
		s.flushMetatableInvalidationBatch()
		clear(batch)
		s.metatableInvalidationBatch = batch
	}
	s.forgetMetatableTypeDependenciesFrom(uri)
	s.dropURICache(uri)

	if !s.IsCI {
		WriteMessage(s.Writer, OutgoingNotification{
			RPC:    "2.0",
			Method: "textDocument/publishDiagnostics",
			Params: PublishDiagnosticsParams{
				URI:         uri,
				Diagnostics: []Diagnostic{},
			},
		})
	}
}

func (s *Server) isGlobalIndexResourceReferencedByDocument(resource ResourceURI, closingURI string) bool {
	if s == nil || resource == "" {
		return false
	}

	for uri, doc := range s.Documents {
		if uri == closingURI || doc == nil {
			continue
		}
		if otherResource, _ := s.globalIndexContext(doc); otherResource == resource {
			return true
		}
	}
	return false
}

func (s *Server) dropURICache(uri string) {
	if s == nil {
		return
	}
	delete(s.uriCache, uri)
	path := s.uriToPath(uri)
	if path != "" {
		delete(s.symlinkCache, filepath.Dir(path))
	}
}

func (s *Server) compileIgnorePatterns() {
	s.compiledIgnores = make([]IgnorePattern, 0, len(s.IgnoreGlobs))

	for _, g := range s.IgnoreGlobs {
		cleanGlob := strings.TrimPrefix(strings.TrimPrefix(g, "**/"), "*/")
		cleanGlob = strings.TrimSuffix(strings.TrimSuffix(cleanGlob, "/**"), "/*")

		if cleanGlob == "" {
			continue
		}

		if !strings.ContainsAny(cleanGlob, "*?") {
			cleanPath := filepath.FromSlash(cleanGlob)

			s.compiledIgnores = append(s.compiledIgnores, IgnorePattern{
				ContainsPath: string(filepath.Separator) + cleanPath + string(filepath.Separator),
				SuffixPath:   string(filepath.Separator) + cleanPath,
				HasSuffix:    cleanGlob,
			})
		} else if strings.HasPrefix(cleanGlob, "*") && !strings.ContainsAny(cleanGlob[1:], "*?") {
			s.compiledIgnores = append(s.compiledIgnores, IgnorePattern{HasSuffix: cleanGlob[1:]})
		} else if strings.HasSuffix(cleanGlob, "*") && !strings.ContainsAny(cleanGlob[:len(cleanGlob)-1], "*?") {
			s.compiledIgnores = append(s.compiledIgnores, IgnorePattern{HasPrefix: cleanGlob[:len(cleanGlob)-1]})
		} else {
			s.compiledIgnores = append(s.compiledIgnores, IgnorePattern{MatchFallback: g})
		}
	}
}

func (s *Server) isIgnored(fullPath, name string) bool {
	if name == "fxmanifest.lua" || name == "__resource.lua" {
		return false
	}

	for _, p := range s.compiledIgnores {
		if p.HasSuffix != "" && strings.HasSuffix(name, p.HasSuffix) {
			return true
		}

		if p.HasPrefix != "" && strings.HasPrefix(name, p.HasPrefix) {
			return true
		}

		if p.ContainsPath != "" && strings.Contains(fullPath, p.ContainsPath) {
			return true
		}

		if p.SuffixPath != "" && strings.HasSuffix(fullPath, p.SuffixPath) {
			return true
		}

		if p.MatchFallback != "" {
			if matched, _ := filepath.Match(p.MatchFallback, name); matched {
				return true
			}
		}
	}

	return false
}

func (s *Server) isIgnoredURI(uri string) bool {
	if doc, ok := s.Documents[uri]; ok {
		return s.isIgnored(doc.Path, filepath.Base(doc.Path))
	}

	path := s.uriToPath(uri)

	if path == "" {
		return false
	}

	return s.isIgnored(path, filepath.Base(path))
}

// pathWithinDirectory compares clean paths at a directory boundary. Both paths
// must use the OS separator and the same case normalization.
func pathWithinDirectory(path, root string) bool {
	if root == "" || !strings.HasPrefix(path, root) {
		return false
	}
	return len(path) == len(root) || root[len(root)-1] == filepath.Separator || path[len(root)] == filepath.Separator
}

func (s *Server) checkIsLibrary(uri, lowerPath string) bool {
	if strings.HasPrefix(uri, embeddedStdlibURIPrefix) {
		return true
	}

	if lowerPath == "" {
		return false
	}

	for _, libPath := range s.lowerLibraryPaths {
		if pathWithinDirectory(lowerPath, libPath) {
			return true
		}
	}

	return false
}

func (s *Server) checkIsWorkspace(uri, lowerPath string) bool {
	if strings.HasPrefix(uri, embeddedStdlibURIPrefix) {
		return false
	}

	if lowerPath == "" {
		return false
	}

	for _, libPath := range s.lowerLibraryPaths {
		if pathWithinDirectory(lowerPath, libPath) {
			return false
		}
	}

	if len(s.lowerWorkspaceFolders) == 0 {
		return true
	}

	for _, wf := range s.lowerWorkspaceFolders {
		if pathWithinDirectory(lowerPath, wf) {
			return true
		}
	}

	return false
}

func (s *Server) isWorkspaceURI(uri string) bool {
	if doc, ok := s.Documents[uri]; ok {
		return doc.IsWorkspace
	}

	return s.checkIsWorkspace(uri, strings.ToLower(s.uriToPath(uri)))
}

func (s *Server) uriToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}

	path := uri[7:]

	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}

	return filepath.Clean(filepath.FromSlash(path))
}

func (s *Server) pathToURI(pathStr string) string {
	cleanPath := filepath.Clean(pathStr)

	if runtime.GOOS == "windows" {
		if len(cleanPath) > 1 && cleanPath[1] == ':' {
			cleanPath = strings.ToLower(cleanPath[:1]) + cleanPath[1:]
		}

		return "file:///" + filepath.ToSlash(cleanPath)
	}

	return "file://" + filepath.ToSlash(cleanPath)
}

func (s *Server) computeModuleName(uri, path, lowerPath string) string {
	if strings.HasPrefix(uri, embeddedStdlibURIPrefix) {
		name := strings.TrimPrefix(uri, embeddedStdlibURIPrefix)

		name = strings.TrimSuffix(name, ".lua")

		return strings.ReplaceAll(name, "/", ".")
	}

	if path == "" {
		return ""
	}

	var (
		bestRoot string
		bestLen  int
	)

	for i, wf := range s.lowerWorkspaceFolders {
		if pathWithinDirectory(lowerPath, wf) {
			if len(wf) > bestLen {
				bestLen = len(wf)
				bestRoot = s.uriToPath(s.WorkspaceFolders[i])
			}
		}
	}

	for i, lib := range s.lowerLibraryPaths {
		if pathWithinDirectory(lowerPath, lib) {
			if len(lib) > bestLen {
				bestLen = len(lib)
				bestRoot = s.LibraryPaths[i]
			}
		}
	}

	if bestRoot != "" {
		rel, err := filepath.Rel(bestRoot, path)
		if err == nil {
			rel = filepath.ToSlash(rel)

			rel = strings.TrimSuffix(rel, ".lua")
			rel = strings.TrimSuffix(rel, "/init")

			return strings.ReplaceAll(rel, "/", ".")
		}
	}

	base := filepath.Base(path)

	base = strings.TrimSuffix(base, ".lua")

	return base
}

const (
	maxURICacheEntries     = 4096
	maxSymlinkCacheEntries = 1024
)

func (s *Server) normalizeURI(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}

	if s.uriCache == nil {
		s.uriCache = make(map[string]string, 1024)
	} else if res, ok := s.uriCache[uri]; ok {
		return res
	}

	path := s.uriToPath(uri)
	dir := filepath.Dir(path)

	if s.symlinkCache == nil {
		s.symlinkCache = make(map[string]string, 256)
	}

	realDir, ok := s.symlinkCache[dir]
	if !ok {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			realDir = r
		} else {
			realDir = dir
		}

		if len(s.symlinkCache) >= maxSymlinkCacheEntries {
			for cachedDir := range s.symlinkCache {
				delete(s.symlinkCache, cachedDir)
				break
			}
		}
		s.symlinkCache[dir] = realDir
	}

	path = filepath.Join(realDir, filepath.Base(path))

	res := s.pathToURI(path)

	if len(s.uriCache) >= maxURICacheEntries {
		for cachedURI := range s.uriCache {
			delete(s.uriCache, cachedURI)
			break
		}
	}
	s.uriCache[uri] = res

	return res
}

func (s *Server) resolveModule(currentURI string, modName string) *Document {
	if modName == "" {
		return nil
	}

	for _, d := range s.Documents {
		if d.ModuleName == modName {
			return d
		}
	}

	modPath := strings.ReplaceAll(modName, ".", "/")

	suffix1 := "/" + modPath + ".lua"
	suffix2 := "/" + modPath + "/init.lua"

	var (
		bestMatch *Document
		bestScore int
	)

	currentDir := ""
	if currentURI != "" {
		if doc, ok := s.Documents[currentURI]; ok {
			currentDir = filepath.ToSlash(doc.Dir)
		} else {
			currentDir = filepath.ToSlash(filepath.Dir(s.uriToPath(currentURI)))
		}
	}

	var rootDirs []string

	for _, wf := range s.WorkspaceFolders {
		rootDirs = append(rootDirs, filepath.ToSlash(s.uriToPath(wf)))
	}

	for _, d := range s.Documents {
		if strings.HasPrefix(d.URI, embeddedStdlibURIPrefix) {
			continue
		}

		path := filepath.ToSlash(d.Path)

		if strings.HasSuffix(path, suffix1) || strings.HasSuffix(path, suffix2) {
			score := 1

			if currentDir != "" && pathWithinDirectory(filepath.FromSlash(path), filepath.FromSlash(currentDir)) {
				score = 3
			} else {
				for _, rootDir := range rootDirs {
					if pathWithinDirectory(filepath.FromSlash(path), filepath.FromSlash(rootDir)) {
						score = 2
						break
					}
				}
			}

			if score > bestScore {
				bestScore = score
				bestMatch = d
			}
		}
	}

	return bestMatch
}

// sendProgressNotification sends a $/progress notification to the client.
func (s *Server) sendProgressNotification(token string, value any) {
	if !s.workDoneProgressSupport {
		return
	}

	params := ProgressParams{
		Token: token,
		Value: value,
	}

	WriteMessage(s.Writer, OutgoingNotification{
		RPC:    "2.0",
		Method: "$/progress",
		Params: params,
	})
}
