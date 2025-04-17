package service_discovery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wenlng/service-discovery/golang/load_balancer"
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

type ServiceDiscoveryType string

// ServiceDiscoveryType .
const (
	ServiceDiscoveryTypeEtcd      ServiceDiscoveryType = "etcd"
	ServiceDiscoveryTypeZookeeper                      = "zookeeper"
	ServiceDiscoveryTypeConsul                         = "consul"
	ServiceDiscoveryTypeNacos                          = "nacos"
	ServiceDiscoveryTypeNone                           = "none"
)

type ServiceDiscoveryLogType string

const (
	ServiceDiscoveryLogTypeWarn  ServiceDiscoveryLogType = "warn"
	ServiceDiscoveryLogTypeInfo                          = "info"
	ServiceDiscoveryLogTypeError                         = "error"
)

type LogOutputHookFunc = func(logType ServiceDiscoveryLogType, message string)

// ServiceDiscovery defines the interface for service discovery
type ServiceDiscovery interface {
	Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error
	Deregister(ctx context.Context, serviceName, instanceID string) error
	Watch(ctx context.Context, serviceName string) (chan []types.Instance, error)
	GetInstances(serviceName string) ([]types.Instance, error)
	SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc)
	Close() error
}

// Config .
type Config struct {
	Type        ServiceDiscoveryType // etcd, zookeeper, consul, nacos, none
	Addrs       string               // 127.0.0.1:8080,192.168.0.1:8080
	TTL         time.Duration        // TTL
	KeepAlive   time.Duration        // Heartbeat interval
	ServiceName string
}

// DiscoveryWithLB .
type DiscoveryWithLB struct {
	discovery ServiceDiscovery
	lb        load_balancer.LoadBalancer
	instances map[string][]types.Instance
	mu        sync.RWMutex
}

// NewServiceDiscovery .
func NewServiceDiscovery(config Config) (ServiceDiscovery, error) {
	var discovery ServiceDiscovery
	var err error
	switch config.Type {
	case ServiceDiscoveryTypeEtcd:
		discovery, err = NewEtcdDiscovery(config.Addrs, config.TTL, config.KeepAlive)
	case ServiceDiscoveryTypeZookeeper:
		discovery, err = NewZooKeeperDiscovery(config.Addrs, config.TTL, config.KeepAlive)
	case ServiceDiscoveryTypeConsul:
		discovery, err = NewConsulDiscovery(config.Addrs, config.TTL, config.KeepAlive)
	case ServiceDiscoveryTypeNacos:
		discovery, err = NewNacosDiscovery(config.Addrs, config.TTL, config.KeepAlive)
	case ServiceDiscoveryTypeNone:
		discovery = &NoopDiscovery{}
	default:
		return nil, fmt.Errorf("unsupported service discovery type: %s", config.Type)
	}
	if err != nil {
		return nil, err
	}

	return discovery, nil
}

// NewDiscoveryWithLB .
func NewDiscoveryWithLB(config Config, lbType load_balancer.LoadBalancerType) (*DiscoveryWithLB, error) {
	discovery, err := NewServiceDiscovery(config)
	if err != nil {
		return nil, err
	}
	var lb load_balancer.LoadBalancer
	switch lbType {
	case load_balancer.LoadBalancerTypeRandom:
		lb = load_balancer.NewRandomBalancer()
	case load_balancer.LoadBalancerTypeRoundRobin:
		lb = load_balancer.NewRoundRobinBalancer()
	case load_balancer.LoadBalancerTypeConsistentHash:
		lb = load_balancer.NewConsistentHashBalancer()
	default:
		return nil, fmt.Errorf("unsupported load balancer type: %s", lbType)
	}
	dlb := &DiscoveryWithLB{
		discovery: discovery,
		lb:        lb,
		instances: make(map[string][]types.Instance),
	}
	return dlb, nil
}

// SetLogOutputHookFunc .
func (d *DiscoveryWithLB) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
	d.discovery.SetLogOutputHookFunc(logOutputHookFunc)
}

// Register .
func (d *DiscoveryWithLB) Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error {
	return d.discovery.Register(ctx, serviceName, instanceID, addr, httpPort, grpcPort)
}

// Deregister .
func (d *DiscoveryWithLB) Deregister(ctx context.Context, serviceName, instanceID string) error {
	return d.discovery.Deregister(ctx, serviceName, instanceID)
}

// Watch .
func (d *DiscoveryWithLB) Watch(ctx context.Context, serviceName string) (chan []types.Instance, error) {
	ch, err := d.discovery.Watch(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	outCh := make(chan []types.Instance)
	go func() {
		for instances := range ch {
			d.mu.Lock()
			d.instances[serviceName] = instances
			d.mu.Unlock()
			outCh <- instances
		}
		close(outCh)
	}()
	return outCh, nil
}

// GetInstances .
func (d *DiscoveryWithLB) GetInstances(serviceName string) ([]types.Instance, error) {
	d.mu.RLock()
	instances, exists := d.instances[serviceName]
	d.mu.RUnlock()
	if exists {
		return instances, nil
	}
	instances, err := d.discovery.GetInstances(serviceName)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.instances[serviceName] = instances
	d.mu.Unlock()
	return instances, nil
}

// Select .
func (d *DiscoveryWithLB) Select(serviceName, key string) (types.Instance, error) {
	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return types.Instance{}, err
	}
	if len(instances) == 0 {
		return types.Instance{}, fmt.Errorf("no instances available for service: %s", serviceName)
	}
	return d.lb.Select(instances, key)
}

// Close .
func (d *DiscoveryWithLB) Close() error {
	return d.discovery.Close()
}

// splitHostPort .
func splitHostPort(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", addr)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", parts[1])
	}
	return parts[0], port, nil
}
