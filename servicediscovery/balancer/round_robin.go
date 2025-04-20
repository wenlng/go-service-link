package balancer

/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

import (
	"fmt"
	"sync/atomic"

	"github.com/wenlng/go-service-link/servicediscovery/instance"
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
func (b *RoundRobinBalancer) Select(instances []instance.ServiceInstance, key string) (instance.ServiceInstance, error) {
	if len(instances) == 0 {
		return instance.ServiceInstance{}, fmt.Errorf("no instances available")
	}
	index := atomic.AddUint64(&b.counter, 1) % uint64(len(instances))
	return instances[index], nil
}
