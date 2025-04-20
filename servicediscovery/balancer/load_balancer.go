/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package balancer

import (
	"github.com/wenlng/go-service-link/servicediscovery/instance"
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
	Select(instances []instance.ServiceInstance, key string) (instance.ServiceInstance, error)
}
