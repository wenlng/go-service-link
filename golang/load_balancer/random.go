package load_balancer

import (
	"fmt"
	"math/rand"

	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

// RandomBalancer implements random load balancing
type RandomBalancer struct{}

func NewRandomBalancer() LoadBalancer {
	return &RandomBalancer{}
}

// Select .
func (b *RandomBalancer) Select(instances []types.Instance, key string) (types.Instance, error) {
	if len(instances) == 0 {
		return types.Instance{}, fmt.Errorf("no instances available")
	}
	return instances[rand.Intn(len(instances))], nil
}
