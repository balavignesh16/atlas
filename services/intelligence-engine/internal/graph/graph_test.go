package graph_test

import (
	"sync"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/graph"
)

func TestAddDependencyAndAggregation(t *testing.T) {
	g := graph.NewDependencyGraph(300)

	// Add 10 identical calls
	for i := 0; i < 10; i++ {
		g.AddDependency("ServiceA", "ServiceB", 100, false, "OK")
	}

	snapshot := g.GetSnapshot()
	if len(snapshot.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(snapshot.Nodes))
	}
	if len(snapshot.Edges) != 1 {
		t.Fatalf("Expected 1 edge, got %d", len(snapshot.Edges))
	}

	edge := snapshot.Edges[0]
	if edge.SourceService != "ServiceA" || edge.TargetService != "ServiceB" {
		t.Errorf("Invalid edge source/target")
	}
	if edge.CallCount != 10 {
		t.Errorf("Expected 10 calls, got %d", edge.CallCount)
	}
	if edge.AverageDurationMs != 100 {
		t.Errorf("Expected 100ms average, got %d", edge.AverageDurationMs)
	}
	if edge.ErrorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", edge.ErrorCount)
	}
}

func TestIgnoreSelfDependency(t *testing.T) {
	g := graph.NewDependencyGraph(300)

	g.AddDependency("atlas-order-service", "atlas-order-service", 100, false, "OK")
	g.AddDependency("atlas-gateway", "atlas-gateway", 100, false, "OK")
	
	g.AddDependency("atlas-gateway", "atlas-order-service", 10, false, "OK")
	g.AddDependency("atlas-order-service", "atlas-payment-service", 20, false, "OK")
	g.AddDependency("atlas-order-service", "atlas-inventory-service", 15, false, "OK")

	snapshot := g.GetSnapshot()
	if len(snapshot.Edges) != 3 {
		t.Fatalf("Expected exactly 3 valid cross-service edges, got %d", len(snapshot.Edges))
	}

	for _, edge := range snapshot.Edges {
		if edge.SourceService == edge.TargetService {
			t.Errorf("Found invalid self-edge: %s -> %s", edge.SourceService, edge.TargetService)
		}
	}
}

func TestGraphExpiration(t *testing.T) {
	g := graph.NewDependencyGraph(1) // 1 second retention

	g.AddDependency("ServiceA", "ServiceB", 100, false, "OK")
	if len(g.GetSnapshot().Edges) != 1 {
		t.Fatalf("Expected 1 edge")
	}

	// Wait for expiration
	time.Sleep(1200 * time.Millisecond)
	g.CleanupExpired(time.Now())

	if len(g.GetSnapshot().Edges) != 0 {
		t.Fatalf("Expected edge to be expired")
	}
}

func TestGraphConcurrency(t *testing.T) {
	g := graph.NewDependencyGraph(300)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.AddDependency("ServiceA", "ServiceB", 10, false, "OK")
			g.GetSnapshot()
		}()
	}

	wg.Wait()
	if len(g.GetSnapshot().Edges) != 1 {
		t.Fatalf("Expected exactly 1 aggregated edge")
	}
}
