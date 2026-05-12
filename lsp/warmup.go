package lsp

import (
	"context"
	"sync"
	"time"
)

const defaultWarmupRate = 10

type WarmupOrchestrator struct {
	mu      sync.Mutex
	workers int
	cancel  context.CancelFunc
	runID   uint64
}

func NewWarmupOrchestrator() *WarmupOrchestrator {
	return &WarmupOrchestrator{workers: 2}
}

func (orchestrator *WarmupOrchestrator) SetWorkers(n int) {
	if orchestrator == nil {
		return
	}
	if n <= 0 {
		n = 2
	}

	orchestrator.mu.Lock()
	orchestrator.workers = n
	orchestrator.mu.Unlock()
}

func (orchestrator *WarmupOrchestrator) Start(uris []ResourceURI, indexFn func(uri ResourceURI)) {
	if orchestrator == nil || indexFn == nil {
		return
	}

	orchestrator.mu.Lock()
	if orchestrator.cancel != nil {
		orchestrator.cancel()
	}
	orchestrator.runID++
	runID := orchestrator.runID

	workers := orchestrator.workers
	if workers <= 0 {
		workers = 2
		orchestrator.workers = workers
	}

	ctx, cancel := context.WithCancel(context.Background())
	orchestrator.cancel = cancel
	orchestrator.mu.Unlock()

	jobs := make(chan ResourceURI)
	ticker := time.NewTicker(time.Second / defaultWarmupRate)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case uri, ok := <-jobs:
					if !ok {
						return
					}
					if uri == "" {
						continue
					}
					indexFn(uri)
				}
			}
		}()
	}

	go func() {
		defer ticker.Stop()
		defer close(jobs)
		for _, uri := range uris {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			select {
			case <-ctx.Done():
				return
			case jobs <- uri:
			}
		}
	}()

	go func() {
		wg.Wait()
		orchestrator.mu.Lock()
		if orchestrator.runID == runID {
			orchestrator.cancel = nil
		}
		orchestrator.mu.Unlock()
	}()
}

func (orchestrator *WarmupOrchestrator) Stop() {
	if orchestrator == nil {
		return
	}

	orchestrator.mu.Lock()
	cancel := orchestrator.cancel
	orchestrator.cancel = nil
	orchestrator.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}
