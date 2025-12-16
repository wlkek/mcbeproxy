// Package proxy provides the core UDP proxy functionality.
package proxy

import (
	"math/rand"
	"sync"

	"mcpeserverproxy/internal/config"
)

// LoadBalancer implements load balancing strategies for proxy outbound selection.
// Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7
type LoadBalancer struct {
	roundRobinIndex map[string]int // groupKey -> current round-robin index
	mu              sync.Mutex
}

// NewLoadBalancer creates a new LoadBalancer instance.
func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		roundRobinIndex: make(map[string]int),
	}
}

// Select selects a node from the given list based on the specified strategy.
// Parameters:
//   - nodes: list of healthy nodes to select from
//   - strategy: load balance strategy (least-latency, round-robin, random, least-connections)
//   - sortBy: latency type for sorting (udp, tcp, http) - used by least-latency strategy
//   - groupKey: identifier for round-robin state tracking
//
// Returns nil if nodes is empty.
// Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7
func (lb *LoadBalancer) Select(nodes []*config.ProxyOutbound, strategy, sortBy, groupKey string) *config.ProxyOutbound {
	if len(nodes) == 0 {
		return nil
	}

	// Single node - no selection needed
	if len(nodes) == 1 {
		return nodes[0]
	}

	switch strategy {
	case config.LoadBalanceLeastLatency:
		return lb.selectLeastLatency(nodes, sortBy)
	case config.LoadBalanceRoundRobin:
		return lb.selectRoundRobin(nodes, groupKey)
	case config.LoadBalanceRandom:
		return lb.selectRandom(nodes)
	case config.LoadBalanceLeastConnections:
		return lb.selectLeastConnections(nodes)
	default:
		// Default to least-latency
		return lb.selectLeastLatency(nodes, sortBy)
	}
}

// selectLeastLatency selects the node with the lowest latency based on sortBy type.
// Requirements: 2.1, 2.5, 2.6, 2.7
func (lb *LoadBalancer) selectLeastLatency(nodes []*config.ProxyOutbound, sortBy string) *config.ProxyOutbound {
	if len(nodes) == 0 {
		return nil
	}

	var selected *config.ProxyOutbound
	var minLatency int64 = -1

	for _, node := range nodes {
		var latency int64
		switch sortBy {
		case config.LoadBalanceSortTCP:
			latency = node.TCPLatencyMs
		case config.LoadBalanceSortHTTP:
			latency = node.HTTPLatencyMs
		case config.LoadBalanceSortUDP:
			fallthrough
		default:
			latency = node.UDPLatencyMs
		}

		// Skip nodes with no latency data (0 means not measured)
		if latency <= 0 {
			continue
		}

		if minLatency < 0 || latency < minLatency {
			minLatency = latency
			selected = node
		}
	}

	// If no node has latency data, return the first node
	if selected == nil && len(nodes) > 0 {
		return nodes[0]
	}

	return selected
}

// selectRoundRobin selects nodes in sequential order, cycling through all nodes.
// Requirements: 2.2
func (lb *LoadBalancer) selectRoundRobin(nodes []*config.ProxyOutbound, groupKey string) *config.ProxyOutbound {
	if len(nodes) == 0 {
		return nil
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Get current index for this group
	index := lb.roundRobinIndex[groupKey]

	// Ensure index is within bounds
	if index >= len(nodes) {
		index = 0
	}

	// Select the node at current index
	selected := nodes[index]

	// Advance index for next selection
	lb.roundRobinIndex[groupKey] = (index + 1) % len(nodes)

	return selected
}

// selectRandom randomly selects a node from the list.
// Requirements: 2.3
func (lb *LoadBalancer) selectRandom(nodes []*config.ProxyOutbound) *config.ProxyOutbound {
	if len(nodes) == 0 {
		return nil
	}

	return nodes[rand.Intn(len(nodes))]
}

// selectLeastConnections selects the node with the fewest active connections.
// Requirements: 2.4
func (lb *LoadBalancer) selectLeastConnections(nodes []*config.ProxyOutbound) *config.ProxyOutbound {
	if len(nodes) == 0 {
		return nil
	}

	var selected *config.ProxyOutbound
	var minConns int64 = -1

	for _, node := range nodes {
		connCount := node.GetConnCount()

		if minConns < 0 || connCount < minConns {
			minConns = connCount
			selected = node
		}
	}

	return selected
}

// ResetRoundRobin resets the round-robin index for a specific group.
// This is useful when the node list changes.
func (lb *LoadBalancer) ResetRoundRobin(groupKey string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	delete(lb.roundRobinIndex, groupKey)
}

// ResetAllRoundRobin resets all round-robin indices.
func (lb *LoadBalancer) ResetAllRoundRobin() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.roundRobinIndex = make(map[string]int)
}

// GetRoundRobinIndex returns the current round-robin index for a group (for testing).
func (lb *LoadBalancer) GetRoundRobinIndex(groupKey string) int {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.roundRobinIndex[groupKey]
}
