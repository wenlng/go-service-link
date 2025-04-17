package load_balancer

import (
	"fmt"
	"hash/crc32"
	"sync"

	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

// ConsistentHashBalancer .
type ConsistentHashBalancer struct {
	hashRing []uint32
	nodes    map[uint32]types.Instance
	mu       sync.RWMutex
}

// NewConsistentHashBalancer .
func NewConsistentHashBalancer() LoadBalancer {
	return &ConsistentHashBalancer{
		nodes: make(map[uint32]types.Instance),
	}
}

// Select .
func (b *ConsistentHashBalancer) Select(instances []types.Instance, key string) (types.Instance, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(instances) == 0 {
		return types.Instance{}, fmt.Errorf("no instances available")
	}

	if len(b.hashRing) != len(instances)*10 {
		b.hashRing = nil
		b.nodes = make(map[uint32]types.Instance)
		for _, inst := range instances {
			for i := 0; i < 10; i++ {
				hash := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s:%d", inst.Addr, i)))
				b.hashRing = append(b.hashRing, hash)
				b.nodes[hash] = inst
			}
		}
		for i := 0; i < len(b.hashRing)-1; i++ {
			for j := i + 1; j < len(b.hashRing); j++ {
				if b.hashRing[i] > b.hashRing[j] {
					b.hashRing[i], b.hashRing[j] = b.hashRing[j], b.hashRing[i]
				}
			}
		}
	}

	hash := crc32.ChecksumIEEE([]byte(key))
	for _, h := range b.hashRing {
		if h >= hash {
			return b.nodes[h], nil
		}
	}
	return b.nodes[b.hashRing[0]], nil
}
