package service_discovery

import (
	"context"

	"github.com/wenlng/service-discovery/golang/service_discovery/types"
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
func (n *NoopDiscovery) Watch(ctx context.Context, serviceName string) (chan []types.Instance, error) {
	ch := make(chan []types.Instance)
	close(ch)
	return ch, nil
}

// GetInstances .
func (n *NoopDiscovery) GetInstances(serviceName string) ([]types.Instance, error) {
	return []types.Instance{}, nil
}

// Close .
func (n *NoopDiscovery) Close() error {
	return nil
}
