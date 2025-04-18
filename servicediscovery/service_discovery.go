/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wenlng/go-captcha-discovery/base"
	"github.com/wenlng/go-captcha-discovery/loadbalancer"
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
	Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error
	Deregister(ctx context.Context, serviceName, instanceID string) error
	Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error)
	GetInstances(serviceName string) ([]base.ServiceInstance, error)
	SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc)
	Close() error
}

type DiscoveryConfig struct {
}

// Config .
type Config struct {
	Type        ServiceDiscoveryType // etcd, zookeeper, consul, nacos, none
	ServiceName string

	// CommonBase
	Addrs          string        // 127.0.0.1:8080,192.168.0.1:8080
	TTL            time.Duration // TTL
	KeepAlive      time.Duration // Heartbeat interval
	MaxRetries     int
	BaseRetryDelay time.Duration
	TlsConfig      *base.TLSConfig
	Username       string
	Password       string

	// Extra Config
	ConsulDiscoveryConfig
	EtcdDiscoveryConfig
	NacosDiscoveryConfig
	ZooKeeperDiscoveryConfig
}

// DiscoveryWithLB .
type DiscoveryWithLB struct {
	discovery ServiceDiscovery
	lb        loadbalancer.LoadBalancer
	instances map[string][]base.ServiceInstance
	mu        sync.RWMutex
}

// NewServiceDiscovery .
func NewServiceDiscovery(config Config) (ServiceDiscovery, error) {
	var discovery ServiceDiscovery
	var err error
	switch config.Type {
	case ServiceDiscoveryTypeEtcd:
		cnf := config.EtcdDiscoveryConfig

		cnf.tlsConfig = config.TlsConfig
		cnf.address = strings.Split(config.Addrs, ",")
		cnf.ttl = config.TTL
		cnf.keepAlive = config.KeepAlive
		cnf.maxRetries = config.MaxRetries
		cnf.baseRetryDelay = config.BaseRetryDelay
		cnf.tlsConfig = config.TlsConfig
		cnf.username = config.Username
		cnf.password = config.Password

		discovery, err = NewEtcdDiscovery(cnf)
	case ServiceDiscoveryTypeZookeeper:
		cnf := config.ZooKeeperDiscoveryConfig

		cnf.tlsConfig = config.TlsConfig
		cnf.address = strings.Split(config.Addrs, ",")
		cnf.ttl = config.TTL
		cnf.keepAlive = config.KeepAlive
		cnf.maxRetries = config.MaxRetries
		cnf.baseRetryDelay = config.BaseRetryDelay
		cnf.tlsConfig = config.TlsConfig
		cnf.username = config.Username
		cnf.password = config.Password

		discovery, err = NewZooKeeperDiscovery(cnf)
	case ServiceDiscoveryTypeConsul:
		cnf := config.ConsulDiscoveryConfig

		cnf.tlsConfig = config.TlsConfig
		cnf.address = strings.Split(config.Addrs, ",")
		cnf.ttl = config.TTL
		cnf.keepAlive = config.KeepAlive
		cnf.maxRetries = config.MaxRetries
		cnf.baseRetryDelay = config.BaseRetryDelay
		cnf.tlsConfig = config.TlsConfig
		cnf.username = config.Username
		cnf.password = config.Password

		discovery, err = NewConsulDiscovery(cnf)
	case ServiceDiscoveryTypeNacos:
		cnf := config.NacosDiscoveryConfig

		cnf.tlsConfig = config.TlsConfig
		cnf.address = strings.Split(config.Addrs, ",")
		cnf.ttl = config.TTL
		cnf.keepAlive = config.KeepAlive
		cnf.maxRetries = config.MaxRetries
		cnf.baseRetryDelay = config.BaseRetryDelay
		cnf.tlsConfig = config.TlsConfig
		cnf.username = config.Username
		cnf.password = config.Password

		discovery, err = NewNacosDiscovery(cnf)
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
func NewDiscoveryWithLB(config Config, lbType loadbalancer.LoadBalancerType) (*DiscoveryWithLB, error) {
	discovery, err := NewServiceDiscovery(config)
	if err != nil {
		return nil, err
	}
	var lb loadbalancer.LoadBalancer
	switch lbType {
	case loadbalancer.LoadBalancerTypeRandom:
		lb = loadbalancer.NewRandomBalancer()
	case loadbalancer.LoadBalancerTypeRoundRobin:
		lb = loadbalancer.NewRoundRobinBalancer()
	case loadbalancer.LoadBalancerTypeConsistentHash:
		lb = loadbalancer.NewConsistentHashBalancer()
	default:
		return nil, fmt.Errorf("unsupported load balancer type: %s", lbType)
	}
	dlb := &DiscoveryWithLB{
		discovery: discovery,
		lb:        lb,
		instances: make(map[string][]base.ServiceInstance),
	}
	return dlb, nil
}

// SetLogOutputHookFunc .
func (d *DiscoveryWithLB) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
	d.discovery.SetLogOutputHookFunc(logOutputHookFunc)
}

// Register .
func (d *DiscoveryWithLB) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	return d.discovery.Register(ctx, serviceName, instanceID, host, httpPort, grpcPort)
}

// Deregister .
func (d *DiscoveryWithLB) Deregister(ctx context.Context, serviceName, instanceID string) error {
	return d.discovery.Deregister(ctx, serviceName, instanceID)
}

// Watch .
func (d *DiscoveryWithLB) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	ch, err := d.discovery.Watch(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	outCh := make(chan []base.ServiceInstance)
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
func (d *DiscoveryWithLB) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
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
func (d *DiscoveryWithLB) Select(serviceName, key string) (base.ServiceInstance, error) {
	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return base.ServiceInstance{}, err
	}
	if len(instances) == 0 {
		return base.ServiceInstance{}, fmt.Errorf("no instances available for service: %s", serviceName)
	}
	return d.lb.Select(instances, key)
}

// Close .
func (d *DiscoveryWithLB) Close() error {
	return d.discovery.Close()
}
