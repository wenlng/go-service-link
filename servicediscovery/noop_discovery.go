/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"

	"github.com/wenlng/go-service-discovery/base"
)

type NoopDiscovery struct{}

// SetLogOutputHookFunc .
func (n *NoopDiscovery) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
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
func (n *NoopDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	ch := make(chan []base.ServiceInstance)
	close(ch)
	return ch, nil
}

// GetInstances .
func (n *NoopDiscovery) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
	return []base.ServiceInstance{}, nil
}

// Close .
func (n *NoopDiscovery) Close() error {
	return nil
}
