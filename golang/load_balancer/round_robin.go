package load_balancer

import (
	"fmt"
	"sync/atomic"

	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	counter uint64
}

// NewRoundRobinBalancer .
func NewRoundRobinBalancer() LoadBalancer {
	return &RoundRobinBalancer{}
}

// Select .
func (b *RoundRobinBalancer) Select(instances []types.Instance, key string) (types.Instance, error) {
	if len(instances) == 0 {
		return types.Instance{}, fmt.Errorf("no instances available")
	}
	index := atomic.AddUint64(&b.counter, 1) % uint64(len(instances))
	return instances[index], nil
}
