/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/helper"
)

// ConsulDiscovery .
type ConsulDiscovery struct {
	client            *api.Client
	logOutputHookFunc LogOutputHookFunc
	config            ConsulDiscoveryConfig
}

// ConsulDiscoveryConfig .
type ConsulDiscoveryConfig struct {
	Scheme     string
	PathPrefix string
	Datacenter string
	Transport  *http.Transport
	HttpClient *http.Client
	WaitTime   time.Duration
	Token      string
	TokenFile  string
	Namespace  string
	Partition  string

	address        []string
	ttl            time.Duration
	keepAlive      time.Duration
	maxRetries     int
	baseRetryDelay time.Duration
	tlsConfig      *base.TLSConfig
	username       string
	password       string
}

// NewConsulDiscovery .
func NewConsulDiscovery(config ConsulDiscoveryConfig) (*ConsulDiscovery, error) {
	if config.maxRetries <= 0 {
		config.maxRetries = 3
	}

	if !helper.IsDurationSet(config.baseRetryDelay) {
		config.baseRetryDelay = 500 * time.Millisecond
	}

	client, err := createConsulClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Consul: %v", err)
	}

	return &ConsulDiscovery{
		client: client,
		config: config,
	}, nil
}

// createConsulClient try to create a Consul client that includes a retry mechanism
func createConsulClient(config ConsulDiscoveryConfig) (*api.Client, error) {
	var client *api.Client
	var err error

	cfg := api.DefaultConfig()
	cfg.Address = config.address[0]

	if config.username != "" || config.password != "" {
		cfg.HttpAuth = &api.HttpBasicAuth{
			Username: config.username,
			Password: config.password,
		}
	}

	if config.tlsConfig != nil {
		cfg.TLSConfig = api.TLSConfig{
			Address:  config.tlsConfig.Address,
			CAFile:   config.tlsConfig.CAFile,
			KeyFile:  config.tlsConfig.KeyFile,
			CertFile: config.tlsConfig.CertFile,
		}
	}

	if helper.IsDurationSet(config.WaitTime) {
		cfg.WaitTime = config.WaitTime
	}
	if config.Scheme != "" {
		cfg.Scheme = config.Scheme
	}
	if config.PathPrefix != "" {
		cfg.PathPrefix = config.PathPrefix
	}
	if config.Datacenter != "" {
		cfg.Datacenter = config.Datacenter
	}
	if config.Transport != nil {
		cfg.Transport = config.Transport
	}
	if config.HttpClient != nil {
		cfg.HttpClient = config.HttpClient
	}
	if config.Token != "" {
		cfg.Token = config.Token
	}
	if config.TokenFile != "" {
		cfg.TokenFile = config.TokenFile
	}
	if config.Namespace != "" {
		cfg.Namespace = config.Namespace
	}
	if config.Partition != "" {
		cfg.Partition = config.Partition
	}

	client, err = api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Consul: %v", err)
	}

	for attempt := 1; attempt <= config.maxRetries; attempt++ {
		client, err = api.NewClient(cfg)
		if err == nil {
			return client, nil
		}

		// Calculate the delay with random jitter
		b := 1 << uint(attempt-1)
		delay := time.Duration(float64(config.baseRetryDelay) * float64(b))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay + jitter)
	}

	return nil, fmt.Errorf("after %d attempts, it still couldn't connect to Consul: %v", config.maxRetries, err)
}

// withRetry perform the operation using the retry logic
func (d *ConsulDiscovery) withRetry(operation func() error) error {
	var err error
	for attempt := 1; attempt <= d.config.maxRetries; attempt++ {
		err = operation()
		if err == nil {
			return nil
		}

		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Operation failed. the %d/%d attempt: %v", attempt, d.config.maxRetries, err))

		// Calculate the delay with random jitter
		b := 1 << uint(attempt-1)
		delay := time.Duration(float64(d.config.baseRetryDelay) * float64(b))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay + jitter)

		// If the client fails, try to reconnect
		if attempt < d.config.maxRetries {
			newClient, clientErr := createConsulClient(d.config)
			if clientErr == nil {
				d.client = newClient
				d.outLog(ServiceDiscoveryLogTypeInfo, "Successfully reconnected to Consul")
			}
		}
	}
	return fmt.Errorf("the operation failed after %d attempts: %v", d.config.maxRetries, err)
}

// SetLogOutputHookFunc Set the log out hook function
func (d *ConsulDiscovery) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
	d.logOutputHookFunc = logOutputHookFunc
}

// outLog
func (d *ConsulDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.logOutputHookFunc != nil {
		d.logOutputHookFunc(logType, message)
	}
}

// Register .
func (d *ConsulDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	port, _ := strconv.Atoi(httpPort)
	registration := &api.AgentServiceRegistration{
		ID:      instanceID,
		Name:    serviceName,
		Address: host,
		Port:    port,
		Meta: map[string]string{
			"hostname":  helper.GetHostname(),
			"host":      host,
			"http_port": httpPort,
			"grpc_port": grpcPort,
		},
		Check: &api.AgentServiceCheck{
			TTL:                            (d.config.ttl + time.Second).String(),
			DeregisterCriticalServiceAfter: (d.config.ttl * 2).String(),
		},
	}
	err := d.withRetry(func() error {
		return d.client.Agent().ServiceRegister(registration)
	})
	if err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	go d.keepAliveLoop(ctx, instanceID)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
	return nil
}

// keepAliveLoop .
func (d *ConsulDiscovery) keepAliveLoop(ctx context.Context, instanceID string) {
	ticker := time.NewTicker(d.config.keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := d.withRetry(func() error {
				return d.client.Agent().PassTTL("service:"+instanceID, "keepalive")
			})
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, "Failed to update TTL")
			}
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeInfo, "Stopping keepalive")
			return
		}
	}
}

// Deregister .
func (d *ConsulDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	err := d.withRetry(func() error {
		return d.client.Agent().ServiceDeregister(instanceID)
	})
	if err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *ConsulDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	ch := make(chan []base.ServiceInstance, 1)
	go func() {
		defer close(ch)
		lastIndex := uint64(0)
		for {
			var services []*api.ServiceEntry
			var meta *api.QueryMeta
			err := d.withRetry(func() error {
				var queryErr error
				services, meta, queryErr = d.client.Health().Service(serviceName, "", true, &api.QueryOptions{WaitIndex: lastIndex})
				return queryErr
			})
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to query services: %v", err))
				time.Sleep(time.Second)
				continue
			}

			lastIndex = meta.LastIndex
			instances := d.servicesToInstances(services)
			select {
			case ch <- instances:
			case <-ctx.Done():
				return
			}
			time.Sleep(time.Second)
		}
	}()
	return ch, nil
}

// GetInstances .
func (d *ConsulDiscovery) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
	var services []*api.ServiceEntry
	err := d.withRetry(func() error {
		var queryErr error
		services, _, queryErr = d.client.Health().Service(serviceName, "", true, nil)
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	return d.servicesToInstances(services), nil
}

// servicesToInstances .
func (d *ConsulDiscovery) servicesToInstances(services []*api.ServiceEntry) []base.ServiceInstance {
	var instances []base.ServiceInstance
	for _, svc := range services {
		instances = append(instances, base.ServiceInstance{
			InstanceID: svc.Service.ID,
			Host:       svc.Service.Address,
			Metadata:   svc.Service.Meta,
		})
	}
	return instances
}

// Close .
func (d *ConsulDiscovery) Close() error {
	return nil
}
