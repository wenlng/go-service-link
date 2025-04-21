/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"

	"github.com/wenlng/go-service-link/servicediscovery/instance"
)

// NoopDiscovery ..
type NoopDiscovery struct{}

// SetOutputLogCallback .
func (n *NoopDiscovery) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
}

// Register .
func (n *NoopDiscovery) Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error {
	return nil
}

// Deregister .
func (n *NoopDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	return nil
}

// Watch .
func (n *NoopDiscovery) Watch(ctx context.Context, serviceName string) (chan []instance.ServiceInstance, error) {
	ch := make(chan []instance.ServiceInstance)
	close(ch)
	return ch, nil
}

// GetInstances .
func (n *NoopDiscovery) GetInstances(serviceName string) ([]instance.ServiceInstance, error) {
	return []instance.ServiceInstance{}, nil
}

// Close .
func (n *NoopDiscovery) Close() error {
	return nil
}
