/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package loadbalancer

import (
	"fmt"
	"math/rand"

	"github.com/wenlng/go-captcha-discovery/base"
)

// RandomBalancer implements random load balancing
type RandomBalancer struct{}

func NewRandomBalancer() LoadBalancer {
	return &RandomBalancer{}
}

// Select .
func (b *RandomBalancer) Select(instances []base.ServiceInstance, key string) (base.ServiceInstance, error) {
	if len(instances) == 0 {
		return base.ServiceInstance{}, fmt.Errorf("no instances available")
	}
	return instances[rand.Intn(len(instances))], nil
}
