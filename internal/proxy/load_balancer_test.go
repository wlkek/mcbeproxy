package proxy

import (
	"mcpeserverproxy/internal/config"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genPositiveLatency generates positive latency values (1-10000 ms)
func genPositiveLatency() gopter.Gen {
	return gen.Int64Range(1, 10000)
}

// genNodeWithLatencies generates a ProxyOutbound with specified latencies
func genNodeWithLatencies(name string, tcpLatency, udpLatency, httpLatency int64) *config.ProxyOutbound {
	return &config.ProxyOutbound{
		Name:          name,
		Type:          config.ProtocolShadowsocks,
		Server:        "example.com",
		Port:          1080,
		Enabled:       true,
		Method:        "aes-256-gcm",
		Password:      "test",
		TCPLatencyMs:  tcpLatency,
		UDPLatencyMs:  udpLatency,
		HTTPLatencyMs: httpLatency,
	}
}

// genNodesWithDistinctLatencies generates N nodes with distinct latencies for the specified sort type
func genNodesWithDistinctLatencies(n int, sortBy string) []*config.ProxyOutbound {
	nodes := make([]*config.ProxyOutbound, n)
	for i := 0; i < n; i++ {
		// Create distinct latencies: node i has latency (i+1)*10
		latency := int64((i + 1) * 10)
		var tcp, udp, http int64
		switch sortBy {
		case config.LoadBalanceSortTCP:
			tcp = latency
			udp = 100 // Fixed value
			http = 100
		case config.LoadBalanceSortHTTP:
			tcp = 100
			udp = 100
			http = latency
		default: // UDP
			tcp = 100
			udp = latency
			http = 100
		}
		nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), tcp, udp, http)
	}
	return nodes
}

// **Feature: proxy-load-balancing, Property 4: Least Latency Selection**
// **Validates: Requirements 2.1, 2.5, 2.6, 2.7**
//
// *For any* non-empty set of healthy nodes with distinct latencies, when load_balance is "least-latency",
// the selected node shall have the minimum latency value according to the specified load_balance_sort type.
func TestProperty4_LeastLatencySelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Test UDP latency selection (default)
	properties.Property("selects node with minimum UDP latency", prop.ForAll(
		func(latencies []int64) bool {
			if len(latencies) < 2 {
				return true // Skip trivial cases
			}

			// Create nodes with the generated latencies
			nodes := make([]*config.ProxyOutbound, len(latencies))
			var minLatency int64 = -1
			var expectedNode *config.ProxyOutbound

			for i, lat := range latencies {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, lat, 100)
				if minLatency < 0 || lat < minLatency {
					minLatency = lat
					expectedNode = nodes[i]
				}
			}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceLeastLatency, config.LoadBalanceSortUDP, "test-group")

			if selected == nil {
				t.Logf("Select returned nil for non-empty nodes")
				return false
			}

			// Selected node should have the minimum UDP latency
			if selected.UDPLatencyMs != expectedNode.UDPLatencyMs {
				t.Logf("Expected node with UDP latency %d, got %d", expectedNode.UDPLatencyMs, selected.UDPLatencyMs)
				return false
			}

			return true
		},
		gen.SliceOfN(5, genPositiveLatency()),
	))

	// Test TCP latency selection
	properties.Property("selects node with minimum TCP latency when sortBy is tcp", prop.ForAll(
		func(latencies []int64) bool {
			if len(latencies) < 2 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, len(latencies))
			var minLatency int64 = -1
			var expectedNode *config.ProxyOutbound

			for i, lat := range latencies {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), lat, 100, 100)
				if minLatency < 0 || lat < minLatency {
					minLatency = lat
					expectedNode = nodes[i]
				}
			}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceLeastLatency, config.LoadBalanceSortTCP, "test-group")

			if selected == nil {
				return false
			}

			return selected.TCPLatencyMs == expectedNode.TCPLatencyMs
		},
		gen.SliceOfN(5, genPositiveLatency()),
	))

	// Test HTTP latency selection
	properties.Property("selects node with minimum HTTP latency when sortBy is http", prop.ForAll(
		func(latencies []int64) bool {
			if len(latencies) < 2 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, len(latencies))
			var minLatency int64 = -1
			var expectedNode *config.ProxyOutbound

			for i, lat := range latencies {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, lat)
				if minLatency < 0 || lat < minLatency {
					minLatency = lat
					expectedNode = nodes[i]
				}
			}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceLeastLatency, config.LoadBalanceSortHTTP, "test-group")

			if selected == nil {
				return false
			}

			return selected.HTTPLatencyMs == expectedNode.HTTPLatencyMs
		},
		gen.SliceOfN(5, genPositiveLatency()),
	))

	// Test empty nodes returns nil
	properties.Property("returns nil for empty nodes", prop.ForAll(
		func(_ int) bool {
			lb := NewLoadBalancer()
			selected := lb.Select([]*config.ProxyOutbound{}, config.LoadBalanceLeastLatency, config.LoadBalanceSortUDP, "test-group")
			return selected == nil
		},
		gen.Int(),
	))

	// Test single node returns that node
	properties.Property("returns single node when only one node exists", prop.ForAll(
		func(latency int64) bool {
			node := genNodeWithLatencies("single-node", 100, latency, 100)
			nodes := []*config.ProxyOutbound{node}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceLeastLatency, config.LoadBalanceSortUDP, "test-group")

			return selected != nil && selected.Name == node.Name
		},
		genPositiveLatency(),
	))

	properties.TestingRun(t)
}

// **Feature: proxy-load-balancing, Property 5: Round Robin Cycling**
// **Validates: Requirements 2.2**
//
// *For any* set of N healthy nodes, when load_balance is "round-robin",
// N consecutive selections shall return each node exactly once before cycling back to the first node.
func TestProperty5_RoundRobinCycling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Test that N consecutive selections return each node exactly once
	properties.Property("N selections return each node exactly once", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 2 || nodeCount > 10 {
				return true // Skip edge cases
			}

			// Create N nodes
			nodes := make([]*config.ProxyOutbound, nodeCount)
			for i := 0; i < nodeCount; i++ {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
			}

			lb := NewLoadBalancer()
			groupKey := "test-round-robin"

			// Track which nodes were selected
			selectedNames := make(map[string]int)

			// Make N selections
			for i := 0; i < nodeCount; i++ {
				selected := lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, groupKey)
				if selected == nil {
					t.Logf("Select returned nil on iteration %d", i)
					return false
				}
				selectedNames[selected.Name]++
			}

			// Each node should be selected exactly once
			for _, node := range nodes {
				count, exists := selectedNames[node.Name]
				if !exists || count != 1 {
					t.Logf("Node %s selected %d times, expected 1", node.Name, count)
					return false
				}
			}

			return true
		},
		gen.IntRange(2, 10),
	))

	// Test that round-robin cycles back to first node after N selections
	properties.Property("cycles back to first node after N selections", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 2 || nodeCount > 10 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, nodeCount)
			for i := 0; i < nodeCount; i++ {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
			}

			lb := NewLoadBalancer()
			groupKey := "test-cycle"

			// Get first selection
			firstSelected := lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, groupKey)
			if firstSelected == nil {
				return false
			}

			// Make N-1 more selections to complete the cycle
			for i := 1; i < nodeCount; i++ {
				lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, groupKey)
			}

			// Next selection should be the first node again
			cycledBack := lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, groupKey)
			if cycledBack == nil {
				return false
			}

			return cycledBack.Name == firstSelected.Name
		},
		gen.IntRange(2, 10),
	))

	// Test that different groups have independent round-robin state
	properties.Property("different groups have independent state", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 2 || nodeCount > 5 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, nodeCount)
			for i := 0; i < nodeCount; i++ {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
			}

			lb := NewLoadBalancer()

			// Select from group1
			group1First := lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, "group1")

			// Select from group2 - should also start from first node
			group2First := lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, "group2")

			// Both should select the same first node (index 0)
			return group1First.Name == group2First.Name
		},
		gen.IntRange(2, 5),
	))

	properties.TestingRun(t)
}

// **Feature: proxy-load-balancing, Property 6: Random Selection Coverage**
// **Validates: Requirements 2.3**
//
// *For any* set of healthy nodes, when load_balance is "random",
// over a sufficient number of selections (e.g., 100 * node_count), every healthy node shall be selected at least once.
func TestProperty6_RandomSelectionCoverage(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Test that all nodes are eventually selected
	properties.Property("all nodes are selected over many iterations", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 2 || nodeCount > 5 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, nodeCount)
			for i := 0; i < nodeCount; i++ {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
			}

			lb := NewLoadBalancer()

			// Track which nodes were selected
			selectedNames := make(map[string]bool)

			// Make 100 * nodeCount selections
			iterations := 100 * nodeCount
			for i := 0; i < iterations; i++ {
				selected := lb.Select(nodes, config.LoadBalanceRandom, config.LoadBalanceSortUDP, "test-random")
				if selected == nil {
					return false
				}
				selectedNames[selected.Name] = true
			}

			// All nodes should have been selected at least once
			for _, node := range nodes {
				if !selectedNames[node.Name] {
					t.Logf("Node %s was never selected in %d iterations", node.Name, iterations)
					return false
				}
			}

			return true
		},
		gen.IntRange(2, 5),
	))

	// Test that random selection returns a valid node
	properties.Property("random selection returns valid node", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 1 || nodeCount > 10 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, nodeCount)
			nodeNames := make(map[string]bool)
			for i := 0; i < nodeCount; i++ {
				name := "node-" + string(rune('a'+i))
				nodes[i] = genNodeWithLatencies(name, 100, 100, 100)
				nodeNames[name] = true
			}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceRandom, config.LoadBalanceSortUDP, "test-random")

			if selected == nil {
				return false
			}

			// Selected node should be one of the input nodes
			return nodeNames[selected.Name]
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// **Feature: proxy-load-balancing, Property 7: Least Connections Selection**
// **Validates: Requirements 2.4**
//
// *For any* non-empty set of healthy nodes with distinct connection counts,
// when load_balance is "least-connections", the selected node shall have the minimum connection count.
func TestProperty7_LeastConnectionsSelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Test that node with minimum connections is selected
	properties.Property("selects node with minimum connection count", prop.ForAll(
		func(connCounts []int64) bool {
			if len(connCounts) < 2 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, len(connCounts))
			var minConns int64 = -1
			var expectedNode *config.ProxyOutbound

			for i, conns := range connCounts {
				node := genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
				// Set connection count by incrementing
				for j := int64(0); j < conns; j++ {
					node.IncrConnCount()
				}
				nodes[i] = node

				if minConns < 0 || conns < minConns {
					minConns = conns
					expectedNode = node
				}
			}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceLeastConnections, config.LoadBalanceSortUDP, "test-group")

			if selected == nil {
				return false
			}

			// Selected node should have the minimum connection count
			return selected.GetConnCount() == expectedNode.GetConnCount()
		},
		gen.SliceOfN(5, gen.Int64Range(0, 100)),
	))

	// Test with all nodes having zero connections
	properties.Property("works with all zero connections", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 1 || nodeCount > 10 {
				return true
			}

			nodes := make([]*config.ProxyOutbound, nodeCount)
			for i := 0; i < nodeCount; i++ {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
				// All nodes have 0 connections (default)
			}

			lb := NewLoadBalancer()
			selected := lb.Select(nodes, config.LoadBalanceLeastConnections, config.LoadBalanceSortUDP, "test-group")

			// Should return a valid node
			if selected == nil {
				return false
			}

			// Selected node should have 0 connections
			return selected.GetConnCount() == 0
		},
		gen.IntRange(1, 10),
	))

	// Test that selection updates when connection counts change
	properties.Property("selection changes when connection counts change", prop.ForAll(
		func(_ int) bool {
			// Create 3 nodes
			nodes := make([]*config.ProxyOutbound, 3)
			for i := 0; i < 3; i++ {
				nodes[i] = genNodeWithLatencies("node-"+string(rune('a'+i)), 100, 100, 100)
			}

			lb := NewLoadBalancer()

			// Initially all have 0 connections, first node should be selected
			selected1 := lb.Select(nodes, config.LoadBalanceLeastConnections, config.LoadBalanceSortUDP, "test-group")
			if selected1 == nil {
				return false
			}

			// Add connections to first two nodes
			nodes[0].IncrConnCount()
			nodes[0].IncrConnCount()
			nodes[1].IncrConnCount()
			// nodes[2] still has 0 connections

			// Now node-c (index 2) should be selected
			selected2 := lb.Select(nodes, config.LoadBalanceLeastConnections, config.LoadBalanceSortUDP, "test-group")
			if selected2 == nil {
				return false
			}

			return selected2.Name == "node-c"
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// Additional unit tests for edge cases

func TestLoadBalancer_EmptyNodes(t *testing.T) {
	lb := NewLoadBalancer()

	strategies := []string{
		config.LoadBalanceLeastLatency,
		config.LoadBalanceRoundRobin,
		config.LoadBalanceRandom,
		config.LoadBalanceLeastConnections,
	}

	for _, strategy := range strategies {
		selected := lb.Select([]*config.ProxyOutbound{}, strategy, config.LoadBalanceSortUDP, "test")
		if selected != nil {
			t.Errorf("Expected nil for empty nodes with strategy %s, got %v", strategy, selected)
		}
	}
}

func TestLoadBalancer_SingleNode(t *testing.T) {
	lb := NewLoadBalancer()
	node := genNodeWithLatencies("single", 100, 100, 100)
	nodes := []*config.ProxyOutbound{node}

	strategies := []string{
		config.LoadBalanceLeastLatency,
		config.LoadBalanceRoundRobin,
		config.LoadBalanceRandom,
		config.LoadBalanceLeastConnections,
	}

	for _, strategy := range strategies {
		selected := lb.Select(nodes, strategy, config.LoadBalanceSortUDP, "test")
		if selected == nil || selected.Name != node.Name {
			t.Errorf("Expected single node for strategy %s, got %v", strategy, selected)
		}
	}
}

func TestLoadBalancer_DefaultStrategy(t *testing.T) {
	lb := NewLoadBalancer()

	// Create nodes with different UDP latencies
	nodes := []*config.ProxyOutbound{
		genNodeWithLatencies("node-a", 100, 50, 100), // Lowest UDP latency
		genNodeWithLatencies("node-b", 100, 100, 100),
		genNodeWithLatencies("node-c", 100, 150, 100),
	}

	// Unknown strategy should default to least-latency
	selected := lb.Select(nodes, "unknown-strategy", config.LoadBalanceSortUDP, "test")
	if selected == nil || selected.Name != "node-a" {
		t.Errorf("Expected node-a (lowest UDP latency) for unknown strategy, got %v", selected)
	}
}

func TestLoadBalancer_ResetRoundRobin(t *testing.T) {
	lb := NewLoadBalancer()
	nodes := genNodesWithDistinctLatencies(3, config.LoadBalanceSortUDP)

	// Make some selections
	lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, "test-group")
	lb.Select(nodes, config.LoadBalanceRoundRobin, config.LoadBalanceSortUDP, "test-group")

	// Index should be 2
	if lb.GetRoundRobinIndex("test-group") != 2 {
		t.Errorf("Expected index 2, got %d", lb.GetRoundRobinIndex("test-group"))
	}

	// Reset
	lb.ResetRoundRobin("test-group")

	// Index should be 0
	if lb.GetRoundRobinIndex("test-group") != 0 {
		t.Errorf("Expected index 0 after reset, got %d", lb.GetRoundRobinIndex("test-group"))
	}
}

func TestLoadBalancer_NoLatencyData(t *testing.T) {
	lb := NewLoadBalancer()

	// Create nodes with zero latency (no data)
	nodes := []*config.ProxyOutbound{
		genNodeWithLatencies("node-a", 0, 0, 0),
		genNodeWithLatencies("node-b", 0, 0, 0),
	}

	// Should return first node when no latency data
	selected := lb.Select(nodes, config.LoadBalanceLeastLatency, config.LoadBalanceSortUDP, "test")
	if selected == nil || selected.Name != "node-a" {
		t.Errorf("Expected node-a when no latency data, got %v", selected)
	}
}
