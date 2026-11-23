package lsp

import (
	"sync"

	"github.com/FRFlo/lugo/ast"
)

type fiveMIndexedEvent struct {
	uri   string
	event FiveMEventInfo
}

type fiveMEventIndex struct {
	mu                 sync.Mutex
	version            uint64
	ready              bool
	byName             map[string][]fiveMIndexedEvent
	byHandler          map[string][]fiveMIndexedEvent
	byCallback         map[string][]fiveMIndexedEvent
	selections         map[string]*fiveMResourceSelection
	callbackResources  map[string]bool
	frameworkDispatch  map[string]fiveMFrameworkDispatch
	frameworkComputing map[string]bool
}

// fiveMEventsNamed builds a generation-scoped index over current document
// facts. Document updates and removals already advance metatableIndexVersion.
func (s *Server) fiveMEventsNamed(name string) []fiveMIndexedEvent {
	s.metatableIndexMu.Lock()
	version := s.metatableIndexVersion
	s.metatableIndexMu.Unlock()
	index := &s.fiveMEventIndex
	index.mu.Lock()
	defer index.mu.Unlock()
	if !index.ready || index.version != version {
		byName := make(map[string][]fiveMIndexedEvent)
		byHandler := make(map[string][]fiveMIndexedEvent)
		byCallback := make(map[string][]fiveMIndexedEvent)
		callbackResources := make(map[string]bool)
		for uri, doc := range s.Documents {
			if doc == nil || doc.Tree == nil {
				continue
			}
			for _, event := range doc.FiveMEvents {
				byName[event.Name] = append(byName[event.Name], fiveMIndexedEvent{uri: uri, event: event})
				if event.Kind == FiveMEventAddHandler || event.Kind == FiveMEventRegisterNet || event.Kind == FiveMFrameworkRegisterServerCallback {
					if leaf := doc.fiveMReferenceLeaf(event.HandlerID); leaf != ast.InvalidNode {
						handlerName := doc.fiveMReferenceName(leaf)
						byHandler[handlerName] = append(byHandler[handlerName], fiveMIndexedEvent{uri: uri, event: event})
					}
				}
				if (event.Kind == FiveMEventTriggerLocal || event.Kind == FiveMFrameworkTriggerServerCallback) && event.NodeID != ast.InvalidNode && int(event.NodeID) < len(doc.Tree.Nodes) {
					callID := doc.Tree.Nodes[event.NodeID].Parent
					if callID != ast.InvalidNode && int(callID) < len(doc.Tree.Nodes) {
						call := doc.Tree.Nodes[callID]
						if call.Kind == ast.KindCallExpr {
							if call.Count > 1 {
								if root := s.getDocumentFiveMProfile(doc).ResourceRoot; root != "" {
									callbackResources[root] = true
								}
							}
							for i := uint32(1); i < call.Count && call.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
								arg := doc.Tree.ExtraList[call.Extra+i]
								if leaf := doc.fiveMReferenceLeaf(arg); leaf != ast.InvalidNode {
									name := doc.fiveMReferenceName(leaf)
									byCallback[name] = append(byCallback[name], fiveMIndexedEvent{uri: uri, event: event})
								}
							}
						}
					}
				}
			}
		}
		index.byName = byName
		index.byHandler = byHandler
		index.byCallback = byCallback
		index.callbackResources = callbackResources
		index.selections = make(map[string]*fiveMResourceSelection)
		index.frameworkDispatch = make(map[string]fiveMFrameworkDispatch)
		index.frameworkComputing = make(map[string]bool)
		index.version = version
		index.ready = true
	}
	return index.byName[name]
}

func (s *Server) fiveMResourceHasLocalEventArguments(doc *Document) bool {
	profile := s.getDocumentFiveMProfile(doc)
	if profile.ResourceRoot == "" {
		return false
	}
	s.fiveMEventsNamed("")
	s.fiveMEventIndex.mu.Lock()
	defer s.fiveMEventIndex.mu.Unlock()
	return s.fiveMEventIndex.callbackResources[profile.ResourceRoot]
}

func (s *Server) fiveMEventsForCallback(name string) []fiveMIndexedEvent {
	s.fiveMEventsNamed("")
	s.fiveMEventIndex.mu.Lock()
	defer s.fiveMEventIndex.mu.Unlock()
	return s.fiveMEventIndex.byCallback[name]
}

func (s *Server) fiveMEventsForHandler(name string) []fiveMIndexedEvent {
	s.fiveMEventsNamed("")
	s.fiveMEventIndex.mu.Lock()
	defer s.fiveMEventIndex.mu.Unlock()
	return s.fiveMEventIndex.byHandler[name]
}
