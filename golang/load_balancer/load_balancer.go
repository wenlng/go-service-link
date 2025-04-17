package load_balancer

import (
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

type LoadBalancerType string

// LoadBalancerType .
const (
	LoadBalancerTypeRandom         LoadBalancerType = "random"
	LoadBalancerTypeRoundRobin                      = "round_robin"
	LoadBalancerTypeConsistentHash                  = "consistent_hash"
)

// LoadBalancer load balance strategy
type LoadBalancer interface {
	Select(instances []types.Instance, key string) (types.Instance, error)
}
