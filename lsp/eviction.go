package lsp

import (
	"sync"
	"time"
)

// DefaultEvictionPolicyMaxSources is the default maximum number of workspace sources to keep in the cache.
const DefaultEvictionPolicyMaxSources = 1024

// EvictionEntry represents a resource and its last access timestamp for cache eviction.
type EvictionEntry struct {
	URI       ResourceURI
	Timestamp int64
}

// EvictionPolicy manages the cache eviction for workspace sources based on access frequency and dependencies.
type EvictionPolicy struct {
	maxSources   int
	evictionList []EvictionEntry
	mu           sync.Mutex
}

// NewEvictionPolicy creates a new EvictionPolicy with an optional custom source limit.
func NewEvictionPolicy(maxSources ...int) *EvictionPolicy {
	limit := DefaultEvictionPolicyMaxSources
	if len(maxSources) > 0 && maxSources[0] > 0 {
		limit = maxSources[0]
	}

	return &EvictionPolicy{maxSources: limit}
}

// RegisterEviction records an access event for a resource to update its position in the eviction queue.
func (p *EvictionPolicy) RegisterEviction(uri ResourceURI) {
	if p == nil || uri == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.registerEvictionLocked(uri, time.Now().UnixNano())
}

func (p *EvictionPolicy) registerEvictionLocked(uri ResourceURI, timestamp int64) {
	if p == nil || uri == "" {
		return
	}
	if p.maxSources <= 0 {
		p.maxSources = DefaultEvictionPolicyMaxSources
	}

	for i := range p.evictionList {
		if p.evictionList[i].URI == uri {
			p.evictionList[i].Timestamp = timestamp
			return
		}
	}

	p.evictionList = append(p.evictionList, EvictionEntry{URI: uri, Timestamp: timestamp})
}

// EvictOldest removes the oldest or least important entries from the cache when the source limit is exceeded.
func (p *EvictionPolicy) EvictOldest(index *GlobalIndex, docs map[string]*Document) {
	if p == nil || index == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.maxSources <= 0 {
		p.maxSources = DefaultEvictionPolicyMaxSources
	}
	if len(p.evictionList) <= p.maxSources {
		return
	}

	for len(p.evictionList) > p.maxSources {
		victimIdx := p.oldestVictimIndexLocked(index)
		if victimIdx == -1 {
			return
		}

		victim := p.evictionList[victimIdx].URI
		p.evictionList = append(p.evictionList[:victimIdx], p.evictionList[victimIdx+1:]...)
		p.evictEntryLocked(index, docs, victim)
	}
}

func (p *EvictionPolicy) oldestVictimIndexLocked(index *GlobalIndex) int {
	if p == nil || len(p.evictionList) == 0 {
		return -1
	}

	victimIdx := -1
	victimDeps := -1
	victimTimestamp := int64(0)
	victimURI := ResourceURI("")

	for i, entry := range p.evictionList {
		deps := evictionDependencyCount(index, entry.URI)
		if victimIdx == -1 || deps < victimDeps || (deps == victimDeps && entry.Timestamp < victimTimestamp) || (deps == victimDeps && entry.Timestamp == victimTimestamp && entry.URI < victimURI) {
			victimIdx = i
			victimDeps = deps
			victimTimestamp = entry.Timestamp
			victimURI = entry.URI
		}
	}

	return victimIdx
}

func evictionDependencyCount(index *GlobalIndex, uri ResourceURI) int {
	if index == nil {
		return 0
	}
	if index.DepGraph != nil {
		return len(index.DepGraph.Dependencies[uri])
	}
	if res := index.Resources[uri]; res != nil {
		return len(res.Dependencies)
	}

	return 0
}

func (p *EvictionPolicy) evictEntryLocked(index *GlobalIndex, docs map[string]*Document, uri ResourceURI) {
	if index != nil {
		_ = index.evictSourceLocked(uri)
	}
	if docs != nil {
		delete(docs, string(uri))
	}
}
