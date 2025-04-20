/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package balancer

import (
	"fmt"
	"math/rand"

	"github.com/wenlng/go-service-link/servicediscovery/instance"
)

// RandomBalancer implements random load balancing
type RandomBalancer struct{}

func NewRandomBalancer() LoadBalancer {
	return &RandomBalancer{}
}

// Select .
func (b *RandomBalancer) Select(instances []instance.ServiceInstance, key string) (instance.ServiceInstance, error) {
	if len(instances) == 0 {
		return instance.ServiceInstance{}, fmt.Errorf("no instances available")
	}
	return instances[rand.Intn(len(instances))], nil
}
