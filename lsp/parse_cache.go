package lsp

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
	"github.com/FRFlo/lugo/token"
	_ "modernc.org/sqlite"
)

const parseCacheSchema = 2
const maxParseCacheBytes = 128 << 20

var cacheBuildOnce sync.Once
var cacheBuildID [32]byte
var cacheBuildOK bool

// The executable identifies the parser and embedded library version, including
// development builds whose advertised version may not change.
func parseCacheBuildID() ([32]byte, bool) {
	cacheBuildOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			return
		}
		copy(cacheBuildID[:], h.Sum(nil))
		cacheBuildOK = true
	})
	return cacheBuildID, cacheBuildOK
}

type parseCache struct {
	db    *sql.DB
	build []byte
}

type cachedParse struct {
	Root     ast.NodeID
	Nodes    []ast.Node
	Comments []token.Token
	Extra    []ast.NodeID
	Errors   []parser.ParseError
}

// The database is a disposable cache. A transaction makes version replacement
// atomic and the conditional writes prevent an older running process from
// repopulating a newer database after an upgrade.
func openParseCache(root string) *parseCache {
	build, ok := parseCacheBuildID()
	if !ok {
		return nil
	}
	if root == "" {
		return nil
	}
	dir := filepath.Join(root, ".lugo")
	if os.MkdirAll(dir, 0700) != nil {
		return nil
	}
	// Never replace an existing ignore file or hide unrelated user files.
	if f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600); err == nil {
		_, _ = f.WriteString("/index.sqlite\n/index.sqlite-*\n/.gitignore\n")
		_ = f.Close()
	}
	dbPath := filepath.Join(dir, "index.sqlite")
	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil
	}
	if err = f.Close(); err != nil {
		return nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil
	}
	db.SetMaxOpenConns(1)
	var tx *sql.Tx
	fail := func() *parseCache {
		if tx != nil {
			_ = tx.Rollback()
		}
		_ = db.Close()
		return nil
	}
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fail()
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fail()
	}
	// Cached entries can always be reconstructed after an interrupted write.
	if _, err = db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return fail()
	}
	tx, err = db.Begin()
	if err != nil {
		return fail()
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail()
	}
	if version != parseCacheSchema {
		if _, err = tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS parse_entries; DROP TABLE IF EXISTS semantic_entries; DROP TABLE IF EXISTS cache_metadata; PRAGMA user_version=%d", parseCacheSchema)); err != nil {
			return fail()
		}
	}
	if _, err = tx.Exec("CREATE TABLE IF NOT EXISTS cache_metadata (id INTEGER PRIMARY KEY CHECK (id=1), build BLOB NOT NULL); CREATE TABLE IF NOT EXISTS parse_entries (key BLOB PRIMARY KEY, payload BLOB NOT NULL, checksum BLOB NOT NULL) WITHOUT ROWID; CREATE TABLE IF NOT EXISTS semantic_entries (key BLOB PRIMARY KEY, payload BLOB NOT NULL, checksum BLOB NOT NULL) WITHOUT ROWID"); err != nil {
		return fail()
	}
	var current []byte
	err = tx.QueryRow("SELECT build FROM cache_metadata WHERE id=1").Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return fail()
	}
	if !bytes.Equal(current, build[:]) {
		if _, err = tx.Exec("DELETE FROM parse_entries; DELETE FROM semantic_entries; DELETE FROM cache_metadata"); err != nil {
			return fail()
		}
		if _, err = tx.Exec("INSERT INTO cache_metadata(id,build) VALUES(1,?)", build[:]); err != nil {
			return fail()
		}
	}
	if err = tx.Commit(); err != nil {
		return fail()
	}
	return &parseCache{db: db, build: append([]byte(nil), build[:]...)}
}

type workspaceCacheRoot struct {
	path  string
	cache *parseCache
}

func (s *Server) openWorkspaceCaches() []workspaceCacheRoot {
	folders := s.WorkspaceFolders
	if len(folders) == 0 && s.RootURI != "" {
		folders = []string{s.RootURI}
	}
	roots := make([]workspaceCacheRoot, 0, len(folders))
	seen := make(map[string]bool, len(folders))
	databases := make(map[string]*parseCache, len(folders))
	for _, folder := range folders {
		path := folder
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "file://") {
			path = s.uriToPath(path)
		} else if strings.Contains(path, "://") {
			continue
		}
		path, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		path = filepath.Clean(path)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() || seen[path] {
			continue
		}
		seen[path] = true
		cacheRoot := repositoryCacheRoot(path)
		cache, exists := databases[cacheRoot]
		if !exists {
			cache = openParseCache(cacheRoot)
			databases[cacheRoot] = cache
		}
		roots = append(roots, workspaceCacheRoot{path: path, cache: cache})
	}
	return roots
}

// A workspace may open a subdirectory of a Git checkout. In that case keep
// the cache at the checkout root, including Git worktrees whose .git is a file.
func repositoryCacheRoot(path string) string {
	for dir := path; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return path
		}
	}
}

func cacheForIndexJob(roots []workspaceCacheRoot, job *IndexJob) *parseCache {
	if len(roots) == 0 {
		return nil
	}
	best := -1
	path := filepath.Clean(job.Path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	if !job.IsStd {
		for i := range roots {
			root := roots[i].path
			if runtime.GOOS == "windows" {
				root = strings.ToLower(root)
			}
			if pathWithinDirectory(path, root) && (best < 0 || len(root) > len(roots[best].path)) {
				best = i
			}
		}
	}
	if best < 0 {
		best = 0
	}
	return roots[best].cache
}

// Resolve nested Git repositories before workers start. Their caches belong in
// those repositories even when the editor opened their common parent folder.
func nestedRepositoryCaches(roots []workspaceCacheRoot, jobs []*IndexJob) (map[*IndexJob]*parseCache, []*parseCache) {
	assigned := make(map[*IndexJob]*parseCache, len(jobs))
	byRepository := make(map[string]*parseCache, len(roots))
	for _, root := range roots {
		byRepository[repositoryCacheRoot(root.path)] = root.cache
	}
	// A directory can be visited under overlapping workspace roots. Include the
	// boundary in the memo key so that its fallback remains workspace-specific.
	directoryRoots := make(map[string]string)
	var opened []*parseCache
	for _, job := range jobs {
		assigned[job] = cacheForIndexJob(roots, job)
		if job.IsStd {
			continue
		}
		path := filepath.Clean(job.Path)
		comparePath := path
		if runtime.GOOS == "windows" {
			comparePath = strings.ToLower(comparePath)
		}
		best := -1
		for i, root := range roots {
			boundary := root.path
			if runtime.GOOS == "windows" {
				boundary = strings.ToLower(boundary)
			}
			if pathWithinDirectory(comparePath, boundary) && (best < 0 || len(root.path) > len(roots[best].path)) {
				best = i
			}
		}
		if best < 0 {
			continue // External libraries use the first workspace's cache.
		}
		boundary := roots[best].path
		dir := filepath.Dir(path)
		memoKey := boundary + "\x00" + dir
		repository, ok := directoryRoots[memoKey]
		if !ok {
			repository = repositoryCacheRoot(boundary)
			for current := dir; ; current = filepath.Dir(current) {
				if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
					repository = current
					break
				}
				if current == boundary || filepath.Dir(current) == current {
					break
				}
			}
			directoryRoots[memoKey] = repository
		}
		cache, exists := byRepository[repository]
		if !exists {
			cache = openParseCache(repository)
			byRepository[repository] = cache
			opened = append(opened, cache)
		}
		assigned[job] = cache
	}
	return assigned, opened
}
func (c *parseCache) close() {
	if c != nil {
		_ = c.db.Close()
	}
}
func parseCacheKey(source []byte, maxErrors int) []byte {
	h := sha256.New()
	h.Write(source)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(maxErrors))
	h.Write(b[:])
	return h.Sum(nil)
}
func (c *parseCache) load(key, source []byte, encoding string) (*ast.Tree, []parser.ParseError, bool) {
	if c == nil {
		return nil, nil, false
	}
	var payload, checksum []byte
	err := c.db.QueryRow("SELECT payload, checksum FROM parse_entries WHERE key=? AND EXISTS (SELECT 1 FROM cache_metadata WHERE id=1 AND build=?) AND length(payload)<=?", key, c.build, maxParseCacheBytes).Scan(&payload, &checksum)
	if err != nil {
		return nil, nil, false
	}
	sum := sha256.Sum256(payload)
	if !bytes.Equal(checksum, sum[:]) {
		return nil, nil, false
	}
	var v cachedParse
	if gob.NewDecoder(bytes.NewReader(payload)).Decode(&v) != nil || !validCachedParse(&v, len(source)) {
		return nil, nil, false
	}
	lines := make([]uint32, 1, bytes.Count(source, []byte{'\n'})+1)
	for i, b := range source {
		if b == '\n' {
			lines = append(lines, uint32(i+1))
		}
	}
	tree := &ast.Tree{Source: source, LineOffsets: lines}
	tree.SetPositionEncoding(encoding)
	tree.Root = v.Root
	tree.Nodes = v.Nodes
	tree.Comments = v.Comments
	tree.ExtraList = v.Extra
	return tree, v.Errors, true
}
func validCachedParse(v *cachedParse, sourceLen int) bool {
	if len(v.Nodes) == 0 || int(v.Root) >= len(v.Nodes) || v.Nodes[0] != (ast.Node{Kind: ast.KindInvalid, Start: ^uint32(0), End: ^uint32(0)}) {
		return false
	}
	for _, n := range v.Nodes[1:] {
		if n.Start > n.End || uint64(n.End) > uint64(sourceLen) || uint64(n.Parent) >= uint64(len(v.Nodes)) || uint64(n.Left) >= uint64(len(v.Nodes)) || uint64(n.Right) >= uint64(len(v.Nodes)) || (n.Count > 0 && uint64(n.Extra)+uint64(n.Count) > uint64(len(v.Extra))) || n.Kind > ast.KindFunctionStmt {
			return false
		}
	}
	for _, id := range v.Extra {
		if uint64(id) >= uint64(len(v.Nodes)) {
			return false
		}
	}
	for _, c := range v.Comments {
		if c.Start > c.End || uint64(c.End) > uint64(sourceLen) {
			return false
		}
	}
	for _, e := range v.Errors {
		if e.Start > e.End || uint64(e.End) > uint64(sourceLen) {
			return false
		}
	}
	return true
}
func (c *parseCache) save(key []byte, tree *ast.Tree, errs []parser.ParseError) {
	if c == nil {
		return
	}
	v := cachedParse{Root: tree.Root, Nodes: tree.Nodes, Comments: tree.Comments, Extra: tree.ExtraList, Errors: errs}
	var b bytes.Buffer
	if gob.NewEncoder(&b).Encode(v) != nil || b.Len() > maxParseCacheBytes {
		return
	}
	sum := sha256.Sum256(b.Bytes())
	_, _ = c.db.Exec("INSERT OR REPLACE INTO parse_entries(key,payload,checksum) SELECT ?,?,? WHERE EXISTS (SELECT 1 FROM cache_metadata WHERE id=1 AND build=?)", key, b.Bytes(), sum[:], c.build)
}

// cachedSemantic contains only the file-local result of semantic.Resolve,
// captured before workspace-specific require bindings and synthetic nodes are
// added. The resolver's working maps and arena are deliberately not persisted.
type cachedSemantic struct {
	References      []ast.NodeID
	GlobalRefs      []ast.NodeID
	GlobalDefs      []ast.NodeID
	FieldDefs       []semantic.FieldDef
	PendingFields   []semantic.FieldRef
	DuplicateLocals []ast.NodeID
	LocalDefs       []ast.NodeID
	ShadowedOuter   []semantic.ShadowPair
	Reassignments   []semantic.Reassignment
}

func (c *parseCache) loadSemantic(key []byte, tree *ast.Tree) *semantic.Resolver {
	if c == nil {
		return nil
	}
	var payload, checksum []byte
	err := c.db.QueryRow("SELECT payload, checksum FROM semantic_entries WHERE key=? AND EXISTS (SELECT 1 FROM cache_metadata WHERE id=1 AND build=?) AND length(payload)<=?", key, c.build, maxParseCacheBytes).Scan(&payload, &checksum)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(payload)
	if !bytes.Equal(checksum, sum[:]) {
		return nil
	}
	var v cachedSemantic
	if gob.NewDecoder(bytes.NewReader(payload)).Decode(&v) != nil || !validCachedSemantic(&v, len(tree.Nodes)) {
		return nil
	}
	compactCachedReceiverNames(&v)
	return &semantic.Resolver{Tree: tree, References: v.References, GlobalRefs: v.GlobalRefs, GlobalDefs: v.GlobalDefs, FieldDefs: v.FieldDefs, PendingFields: v.PendingFields, DuplicateLocals: v.DuplicateLocals, LocalDefs: v.LocalDefs, ShadowedOuter: v.ShadowedOuter, Reassignments: v.Reassignments}
}

// Gob restores each byte slice independently. Pack the immutable receiver
// names into one allocation and share repeated names, as the live resolver does
// with its source and name arena.
func compactCachedReceiverNames(v *cachedSemantic) {
	if len(v.FieldDefs)+len(v.PendingFields) == 0 {
		return
	}
	type span struct{ start, end int }
	names := make(map[string]span)
	total := 0
	add := func(name []byte) {
		if len(name) == 0 {
			return
		}
		key := string(name)
		if _, exists := names[key]; !exists {
			names[key] = span{total, total + len(name)}
			total += len(name)
		}
	}
	for _, field := range v.FieldDefs {
		add(field.ReceiverName)
	}
	for _, field := range v.PendingFields {
		add(field.ReceiverName)
	}
	arena := make([]byte, total)
	pack := func(name []byte) []byte {
		if len(name) == 0 {
			return nil
		}
		location := names[string(name)]
		copy(arena[location.start:location.end], name)
		return arena[location.start:location.end:location.end]
	}
	for i := range v.FieldDefs {
		v.FieldDefs[i].ReceiverName = pack(v.FieldDefs[i].ReceiverName)
	}
	for i := range v.PendingFields {
		v.PendingFields[i].ReceiverName = pack(v.PendingFields[i].ReceiverName)
	}
}

func validCachedSemantic(v *cachedSemantic, nodes int) bool {
	if len(v.References) != nodes {
		return false
	}
	valid := func(id ast.NodeID) bool { return uint64(id) < uint64(nodes) }
	for _, ids := range [][]ast.NodeID{v.References, v.GlobalRefs, v.GlobalDefs, v.DuplicateLocals, v.LocalDefs} {
		for _, id := range ids {
			if !valid(id) {
				return false
			}
		}
	}
	for _, f := range v.FieldDefs {
		if !valid(f.ReceiverDef) || !valid(f.NodeID) {
			return false
		}
	}
	for _, f := range v.PendingFields {
		if !valid(f.ReceiverDef) || !valid(f.PropNodeID) {
			return false
		}
	}
	for _, p := range v.ShadowedOuter {
		if !valid(p.Shadowing) || !valid(p.Shadowed) {
			return false
		}
	}
	for _, r := range v.Reassignments {
		if !valid(r.DefID) || !valid(r.ValID) {
			return false
		}
	}
	return true
}

func (c *parseCache) saveSemantic(key []byte, r *semantic.Resolver) {
	if c == nil {
		return
	}
	v := cachedSemantic{References: r.References, GlobalRefs: r.GlobalRefs, GlobalDefs: r.GlobalDefs, FieldDefs: r.FieldDefs, PendingFields: r.PendingFields, DuplicateLocals: r.DuplicateLocals, LocalDefs: r.LocalDefs, ShadowedOuter: r.ShadowedOuter, Reassignments: r.Reassignments}
	var b bytes.Buffer
	if gob.NewEncoder(&b).Encode(v) != nil || b.Len() > maxParseCacheBytes {
		return
	}
	sum := sha256.Sum256(b.Bytes())
	_, _ = c.db.Exec("INSERT OR REPLACE INTO semantic_entries(key,payload,checksum) SELECT ?,?,? WHERE EXISTS (SELECT 1 FROM cache_metadata WHERE id=1 AND build=?)", key, b.Bytes(), sum[:], c.build)
}
