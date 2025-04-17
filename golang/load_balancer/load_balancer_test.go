package load_balancer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

func TestLoadBalancers(t *testing.T) {
	instances := []types.Instance{
		{Addr: "localhost:8081", Metadata: map[string]string{"http_port": "8081"}},
		{Addr: "localhost:8082", Metadata: map[string]string{"http_port": "8082"}},
		{Addr: "localhost:8083", Metadata: map[string]string{"http_port": "8083"}},
	}

	t.Run("RandomBalancer", func(t *testing.T) {
		lb := NewRandomBalancer()
		counts := make(map[string]int)
		for i := 0; i < 1000; i++ {
			inst, err := lb.Select(instances, "")
			assert.NoError(t, err)
			counts[inst.Addr]++
		}
		for _, addr := range []string{"localhost:8081", "localhost:8082", "localhost:8083"} {
			assert.Greater(t, counts[addr], 200, "Random balancer should distribute requests")
		}
	})

	t.Run("RoundRobinBalancer", func(t *testing.T) {
		lb := NewRoundRobinBalancer()
		for i, expected := range []string{"localhost:8081", "localhost:8082", "localhost:8083", "localhost:8081"} {
			inst, err := lb.Select(instances, "")
			assert.NoError(t, err)
			assert.Equal(t, expected, inst.Addr, "Round robin balancer should select in order at iteration %d", i)
		}
	})

	t.Run("ConsistentHashBalancer", func(t *testing.T) {
		lb := NewConsistentHashBalancer()
		key := "test-key"
		var lastAddr string
		for i := 0; i < 10; i++ {
			inst, err := lb.Select(instances, key)
			assert.NoError(t, err)
			if i == 0 {
				lastAddr = inst.Addr
			} else {
				assert.Equal(t, lastAddr, inst.Addr, "Consistent hash balancer should select same instance for same key")
			}
		}
		inst, err := lb.Select(instances, "different-key")
		assert.NoError(t, err)
		assert.NotEqual(t, lastAddr, inst.Addr, "Consistent hash balancer may select different instance for different key")
	})

	t.Run("EmptyInstances", func(t *testing.T) {
		lbs := []LoadBalancer{
			NewRandomBalancer(),
			NewRoundRobinBalancer(),
			NewConsistentHashBalancer(),
		}
		for _, lb := range lbs {
			_, err := lb.Select([]types.Instance{}, "")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no instances available")
		}
	})
}
