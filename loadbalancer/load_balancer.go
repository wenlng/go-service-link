/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package loadbalancer

import "github.com/wenlng/go-captcha-discovery/base"

type LoadBalancerType string

// LoadBalancerType .
const (
	LoadBalancerTypeRandom         LoadBalancerType = "random"
	LoadBalancerTypeRoundRobin                      = "round_robin"
	LoadBalancerTypeConsistentHash                  = "consistent_hash"
)

// LoadBalancer load balance strategy
type LoadBalancer interface {
	Select(instances []base.ServiceInstance, key string) (base.ServiceInstance, error)
}
