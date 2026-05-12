package lsp

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestWarmupOrchestrator(t *testing.T) {
	orchestrator := NewWarmupOrchestrator()
	orchestrator.SetWorkers(1)

	uris := []ResourceURI{"A", "B", "C"}
	var mu sync.Mutex
	got := make([]ResourceURI, 0, len(uris))
	done := make(chan struct{})

	orchestrator.Start(uris, func(uri ResourceURI) {
		mu.Lock()
		got = append(got, uri)
		if len(got) == len(uris) {
			close(done)
		}
		mu.Unlock()
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("warmup did not finish in time")
	}

	orchestrator.Stop()

	if !slices.Equal(got, uris) {
		t.Fatalf("warmup order = %#v, want %#v", got, uris)
	}
}

func TestWarmupOrchestratorStop(t *testing.T) {
	orchestrator := NewWarmupOrchestrator()
	orchestrator.SetWorkers(2)

	blocked := make(chan struct{})
	finished := make(chan struct{})
	orchestrator.Start([]ResourceURI{"A"}, func(uri ResourceURI) {
		close(blocked)
		<-finished
	})

	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("warmup worker did not start")
	}

	orchestrator.Stop()
	close(finished)
}

func TestWarmupOrchestratorZeroValueDefaults(t *testing.T) {
	orchestrator := &WarmupOrchestrator{}
	seen := make(chan ResourceURI, 1)
	orchestrator.Start([]ResourceURI{"A"}, func(uri ResourceURI) {
		seen <- uri
	})

	select {
	case uri := <-seen:
		if uri != "A" {
			t.Fatalf("warmup uri = %q, want A", uri)
		}
	case <-time.After(time.Second):
		t.Fatal("warmup default workers did not process uri")
	}

	orchestrator.Stop()
}
