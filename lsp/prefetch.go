package lsp

import "sync"

type PrefetchEngine struct {
	mu        sync.Mutex
	Graph     *DependencyGraph
	queue     []ResourceURI
	queued    map[ResourceURI]struct{}
	IndexFunc func(ResourceURI)
}

func NewPrefetchEngine(graph *DependencyGraph, indexFn func(ResourceURI)) *PrefetchEngine {
	return &PrefetchEngine{Graph: graph, IndexFunc: indexFn}
}

func (engine *PrefetchEngine) Enqueue(uri ResourceURI, depth int) {
	if engine == nil || uri == "" || depth < 0 {
		return
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()

	if engine.queued == nil {
		engine.queued = make(map[ResourceURI]struct{})
	}

	engine.enqueueLocked(uri, depth)
}

func (engine *PrefetchEngine) enqueueLocked(uri ResourceURI, depth int) {
	if uri == "" {
		return
	}
	if _, ok := engine.queued[uri]; ok {
		return
	}

	engine.queued[uri] = struct{}{}
	engine.queue = append(engine.queue, uri)

	if depth == 0 || engine.Graph == nil {
		return
	}

	for _, dep := range engine.Graph.DependencyList(uri) {
		engine.enqueueLocked(dep, depth-1)
	}
}

func (engine *PrefetchEngine) ProcessQueue(server *Server) {
	if engine == nil {
		return
	}

	engine.mu.Lock()
	queue := append([]ResourceURI(nil), engine.queue...)
	engine.queue = engine.queue[:0]
	engine.queued = make(map[ResourceURI]struct{})
	engine.mu.Unlock()

	for _, uri := range queue {
		if engine.IndexFunc != nil {
			engine.IndexFunc(uri)
			continue
		}

		if server == nil {
			continue
		}

		doc := server.Documents[string(uri)]
		if doc == nil || doc.Tree == nil {
			continue
		}

		server.updateDocument(string(uri), doc.Source())
	}
}
