/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wenlng/go-service-link/foundation/common"
	"github.com/wenlng/go-service-link/foundation/helper"
	"github.com/wenlng/go-service-link/servicediscovery/balancer"
	"github.com/wenlng/go-service-link/servicediscovery/instance"
)

// registeredServiceInfo Save the information of the registered services
type registeredServiceInfo struct {
	ServiceName string
	InstanceID  string
	Host        string
	HTTPPort    string
	GRPCPort    string
}

// ServiceDiscoveryType .
type ServiceDiscoveryType string

const (
	ServiceDiscoveryTypeEtcd      ServiceDiscoveryType = "etcd"
	ServiceDiscoveryTypeZookeeper                      = "zookeeper"
	ServiceDiscoveryTypeConsul                         = "consul"
	ServiceDiscoveryTypeNacos                          = "nacos"
	ServiceDiscoveryTypeNone                           = "none"
)

// OutputLogType ..
type OutputLogType = helper.OutputLogType

const (
	OutputLogTypeWarn  = helper.OutputLogTypeWarn
	OutputLogTypeInfo  = helper.OutputLogTypeInfo
	OutputLogTypeError = helper.OutputLogTypeError
	OutputLogTypeDebug = helper.OutputLogTypeDebug
)

// OutputLogCallback ..
type OutputLogCallback = helper.OutputLogCallback

// ServiceDiscovery defines the interface for service discovery
type ServiceDiscovery interface {
	Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error
	Deregister(ctx context.Context, serviceName, instanceID string) error
	Watch(ctx context.Context, serviceName string) (chan []instance.ServiceInstance, error)
	GetInstances(serviceName string) ([]instance.ServiceInstance, error)
	SetOutputLogCallback(outputLogCallback OutputLogCallback)
	Close() error
}

// Config .
type Config struct {
	Type        ServiceDiscoveryType // etcd, zookeeper, consul, nacos, none
	ServiceName string

	// CommonBase
	Addrs          string // 127.0.0.1:8080,192.168.0.1:8080
	PoolSize       int
	TTL            time.Duration // TTL
	KeepAlive      time.Duration // Heartbeat interval
	MaxRetries     int
	BaseRetryDelay time.Duration
	TlsConfig      *common.TLSConfig
	Username       string
	Password       string

	// Health Check
	HealthCheckConfig

	// Extra Config
	ConsulDiscoveryConfig
	EtcdDiscoveryConfig
	NacosDiscoveryConfig
	ZooKeeperDiscoveryConfig
}

// NewServiceDiscovery .
func NewServiceDiscovery(config Config) (ServiceDiscovery, error) {
	var discovery ServiceDiscovery
	var err error
	switch config.Type {
	case ServiceDiscoveryTypeEtcd:
		cnf := config.EtcdDiscoveryConfig

		cnf.tlsConfig = config.TlsConfig
		cnf.poolSize = config.PoolSize
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
		cnf.poolSize = config.PoolSize
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
		cnf.poolSize = config.PoolSize
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
		cnf.poolSize = config.PoolSize
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

// HealthCheckConfig .
type HealthCheckConfig struct {
	Interval        time.Duration // How often to check
	Timeout         time.Duration // Timeout for each check
	MaxFailedChecks int           // Max failed checks before marking unhealthy
	MinHealthyTime  time.Duration // Minimum time to consider instance healthy
}

// DiscoveryWithLB .
type DiscoveryWithLB struct {
	discovery ServiceDiscovery
	lb        balancer.LoadBalancer
	instances map[string][]instance.ServiceInstance
	insMu     sync.RWMutex

	healthCheckConfig HealthCheckConfig        // Health check configuration
	healthCheckStop   map[string]chan struct{} // Channels to stop health checks
	hcsMu             sync.RWMutex
}

// NewDiscoveryWithLB .
func NewDiscoveryWithLB(config Config, lbType balancer.LoadBalancerType) (*DiscoveryWithLB, error) {
	discovery, err := NewServiceDiscovery(config)
	if err != nil {
		return nil, err
	}
	var lb balancer.LoadBalancer
	switch lbType {
	case balancer.LoadBalancerTypeRandom:
		lb = balancer.NewRandomBalancer()
	case balancer.LoadBalancerTypeRoundRobin:
		lb = balancer.NewRoundRobinBalancer()
	case balancer.LoadBalancerTypeConsistentHash:
		lb = balancer.NewConsistentHashBalancer()
	default:
		return nil, fmt.Errorf("unsupported load balancer type: %s", lbType)
	}

	if !helper.IsDurationSet(config.HealthCheckConfig.Interval) {
		config.HealthCheckConfig.Interval = 5 * time.Second
	}
	if !helper.IsDurationSet(config.HealthCheckConfig.Timeout) {
		config.HealthCheckConfig.Timeout = 2 * time.Second
	}
	if !helper.IsDurationSet(config.HealthCheckConfig.MinHealthyTime) {
		config.HealthCheckConfig.MinHealthyTime = 30 * time.Second
	}
	if config.HealthCheckConfig.MaxFailedChecks <= 0 {
		config.HealthCheckConfig.MaxFailedChecks = 3
	}

	dlb := &DiscoveryWithLB{
		discovery: discovery,
		lb:        lb,
		instances: make(map[string][]instance.ServiceInstance),
		healthCheckConfig: HealthCheckConfig{
			Interval:        config.HealthCheckConfig.Interval,
			Timeout:         config.HealthCheckConfig.Timeout,
			MaxFailedChecks: config.HealthCheckConfig.MaxFailedChecks,
			MinHealthyTime:  config.HealthCheckConfig.MinHealthyTime,
		},
		healthCheckStop: make(map[string]chan struct{}),
	}
	return dlb, nil
}

// SetOutputLogCallback .
func (d *DiscoveryWithLB) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
	d.discovery.SetOutputLogCallback(outputLogCallback)
}

// Register .
func (d *DiscoveryWithLB) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	err := d.discovery.Register(ctx, serviceName, instanceID, host, httpPort, grpcPort)
	if err == nil {
		d.startHealthCheck(ctx, serviceName, instanceID, host, httpPort)
	}
	return err
}

// Deregister .
func (d *DiscoveryWithLB) Deregister(ctx context.Context, serviceName, instanceID string) error {
	d.stopHealthCheck(serviceName, instanceID)
	return d.discovery.Deregister(ctx, serviceName, instanceID)
}

// Watch .
func (d *DiscoveryWithLB) Watch(ctx context.Context, serviceName string) (chan []instance.ServiceInstance, error) {
	ch, err := d.discovery.Watch(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	outCh := make(chan []instance.ServiceInstance)
	go func() {
		for instances := range ch {
			d.insMu.Lock()
			// Update existing instances with health status
			updatedInstances := make([]instance.ServiceInstance, len(instances))
			for i, inst := range instances {
				existing, exists := d.findInstance(serviceName, inst.InstanceID)
				if exists && existing.IsHealthy {
					inst.IsHealthy = existing.IsHealthy
					inst.LastChecked = existing.LastChecked
					inst.FailedChecks = existing.FailedChecks
				} else {
					inst.IsHealthy = true
					inst.LastChecked = time.Now()
				}
				updatedInstances[i] = inst
				if !exists {
					if inst.HTTPPort != "" {
						d.startHealthCheck(ctx, serviceName, inst.InstanceID, inst.Host, inst.HTTPPort)
					}
					if inst.GRPCPort != "" {
						d.startHealthCheck(ctx, serviceName, inst.InstanceID, inst.Host, inst.GRPCPort)
					}
				}
			}
			d.instances[serviceName] = updatedInstances
			d.insMu.Unlock()
			outCh <- updatedInstances
		}
		close(outCh)
	}()

	return outCh, nil
}

// GetInstances .
func (d *DiscoveryWithLB) GetInstances(serviceName string) ([]instance.ServiceInstance, error) {
	d.insMu.RLock()
	instances, exists := d.instances[serviceName]
	d.insMu.RUnlock()
	if exists {
		return d.filterHealthyInstances(instances), nil
	}

	instances, err := d.discovery.GetInstances(serviceName)
	if err != nil {
		return nil, err
	}

	d.insMu.Lock()
	for i := range instances {
		instances[i].IsHealthy = true
		instances[i].LastChecked = time.Now()

		if instances[i].HTTPPort != "" {
			d.startHealthCheck(context.Background(), serviceName, instances[i].InstanceID, instances[i].Host, instances[i].HTTPPort)
		}
		if instances[i].GRPCPort != "" {
			d.startHealthCheck(context.Background(), serviceName, instances[i].InstanceID, instances[i].Host, instances[i].GRPCPort)
		}
	}
	d.instances[serviceName] = instances
	d.insMu.Unlock()

	return d.filterHealthyInstances(instances), nil
}

// Select .
func (d *DiscoveryWithLB) Select(serviceName, key string) (instance.ServiceInstance, error) {
	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return instance.ServiceInstance{}, err
	}
	if len(instances) == 0 {
		return instance.ServiceInstance{}, fmt.Errorf("no instances available for service: %s", serviceName)
	}
	return d.lb.Select(instances, key)
}

// Close .
func (d *DiscoveryWithLB) Close() error {
	for serviceName := range d.healthCheckStop {
		d.stopAllHealthChecks(serviceName)
	}

	return d.discovery.Close()
}

// startHealthCheck begins periodic health checking for an instance
func (d *DiscoveryWithLB) startHealthCheck(ctx context.Context, serviceName, instanceID, host, httpPort string) {
	key := serviceName + "-" + instanceID
	stopChan := make(chan struct{})
	d.hcsMu.Lock()
	if _, ok := d.healthCheckStop[key]; ok {
		d.hcsMu.Unlock()
		return
	}
	d.healthCheckStop[key] = stopChan
	d.hcsMu.Unlock()

	go func() {
		ticker := time.NewTicker(d.healthCheckConfig.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-ticker.C:
				isHealthy := d.performHealthCheck(host, httpPort)
				var hasInts bool
				d.insMu.Lock()
				instances, exists := d.instances[serviceName]
				if exists {
					for i, inst := range instances {
						if inst.InstanceID == instanceID {
							if isHealthy {
								instances[i].FailedChecks = 0
								instances[i].IsHealthy = true
							} else {
								instances[i].FailedChecks++
								if instances[i].FailedChecks >= d.healthCheckConfig.MaxFailedChecks {
									instances[i].IsHealthy = false
								}
							}
							instances[i].LastChecked = time.Now()
							d.instances[serviceName] = instances
							hasInts = true
							break
						}
					}
				}
				d.insMu.Unlock()

				if !hasInts {
					d.hcsMu.Lock()
					if _, ok := d.healthCheckStop[key]; ok {
						close(d.healthCheckStop[key])
						delete(d.healthCheckStop, key)
					}
					d.hcsMu.Unlock()
				}
			}
		}
	}()
}

// performHealthCheck ..
func (d *DiscoveryWithLB) performHealthCheck(host, httpPort string) bool {
	_, cancel := context.WithTimeout(context.Background(), d.healthCheckConfig.Timeout)
	defer cancel()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", host, httpPort), d.healthCheckConfig.Timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// stopHealthCheck ..
func (d *DiscoveryWithLB) stopHealthCheck(serviceName, instanceID string) {
	d.hcsMu.Lock()
	if stopChan, exists := d.healthCheckStop[serviceName+"-"+instanceID]; exists {
		close(stopChan)
		delete(d.healthCheckStop, serviceName+"-"+instanceID)
	}
	d.hcsMu.Unlock()
}

// stopAllHealthChecks ..
func (d *DiscoveryWithLB) stopAllHealthChecks(serviceName string) {
	d.hcsMu.Lock()
	for key, stopChan := range d.healthCheckStop {
		if strings.HasPrefix(key, serviceName+"-") {
			close(stopChan)
			delete(d.healthCheckStop, key)
		}
	}
	d.hcsMu.Unlock()
}

// findInstance ..
func (d *DiscoveryWithLB) findInstance(serviceName, instanceID string) (instance.ServiceInstance, bool) {
	instances, exists := d.instances[serviceName]
	if !exists {
		return instance.ServiceInstance{}, false
	}
	for _, inst := range instances {
		if inst.InstanceID == instanceID {
			return inst, true
		}
	}
	return instance.ServiceInstance{}, false
}

// filterHealthyInstances ..
func (d *DiscoveryWithLB) filterHealthyInstances(instances []instance.ServiceInstance) []instance.ServiceInstance {
	var healthy []instance.ServiceInstance
	for _, inst := range instances {
		if inst.IsHealthy && time.Since(inst.LastChecked) < d.healthCheckConfig.MinHealthyTime {
			healthy = append(healthy, inst)
		}
	}
	return healthy
}
