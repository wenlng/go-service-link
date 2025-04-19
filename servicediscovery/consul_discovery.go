/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
)

// ConsulDiscovery .
type ConsulDiscovery struct {
	pool              *clientpool.ConsulPool
	outputLogCallback OutputLogCallback
	config            ConsulDiscoveryConfig

	registeredServices map[string]registeredServiceInfo
	mutex              sync.RWMutex
}

// ConsulDiscoveryConfig .
type ConsulDiscoveryConfig struct {
	extraconfig.ConsulExtraConfig

	address        []string
	poolSize       int
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
	if config.poolSize <= 0 {
		config.poolSize = 5
	}

	if config.maxRetries <= 0 {
		config.maxRetries = 3
	}

	if !helper.IsDurationSet(config.baseRetryDelay) {
		config.baseRetryDelay = 500 * time.Millisecond
	}

	pool, err := clientpool.NewConsulPool(config.poolSize, config.address, &config.ConsulExtraConfig)
	if err != nil {
		return nil, err
	}

	return &ConsulDiscovery{
		config:             config,
		pool:               pool,
		registeredServices: make(map[string]registeredServiceInfo),
	}, nil
}

// SetOutputLogCallback Set the log out hook function
func (d *ConsulDiscovery) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
	d.outputLogCallback = outputLogCallback
}

// outLog
func (d *ConsulDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.outputLogCallback != nil {
		d.outputLogCallback(logType, message)
	}
}

// Register .
func (d *ConsulDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

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

	operation := func() error {
		return cli.Agent().ServiceRegister(registration)
	}
	if err := helper.WithRetry(ctx, operation); err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	d.mutex.Lock()
	d.registeredServices[instanceID] = registeredServiceInfo{
		ServiceName: serviceName,
		InstanceID:  instanceID,
		Host:        host,
		HTTPPort:    httpPort,
		GRPCPort:    grpcPort,
	}
	d.mutex.Unlock()

	go d.keepAliveLoop(ctx, instanceID)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[ConsulDiscovery] Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
	return nil
}

// checkAndReRegisterServices Check and re-register the service
func (d *ConsulDiscovery) checkAndReRegisterServices(ctx context.Context) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	d.mutex.RLock()
	services := make(map[string]registeredServiceInfo)
	for k, v := range d.registeredServices {
		services[k] = v
	}
	d.mutex.RUnlock()

	for instanceID, svcInfo := range services {
		operation := func() error {
			services, _, err := cli.Health().Service(svcInfo.ServiceName, "", true, &api.QueryOptions{})
			if err != nil {
				return fmt.Errorf("failed to check the service registration status: %v", err)
			}

			registered := false
			for _, svc := range services {
				if svc.Service.ID == instanceID {
					registered = true
					break
				}
			}

			if !registered {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ConsulDiscovery] The service has not been registered. Re-register: %s, instanceID: %s", svcInfo.ServiceName, instanceID))
				return d.Register(ctx, svcInfo.ServiceName, instanceID, svcInfo.Host, svcInfo.HTTPPort, svcInfo.GRPCPort)
			}
			return nil
		}

		if err := helper.WithRetry(ctx, operation); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ConsulDiscovery] The re-registration service failed: %v", err))
			return err
		}
	}
	return nil
}

// keepAliveLoop .
func (d *ConsulDiscovery) keepAliveLoop(ctx context.Context, instanceID string) {
	ticker := time.NewTicker(d.config.keepAlive)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cli := d.pool.Get()
			operation := func() error {
				return cli.Agent().PassTTL("service:"+instanceID, "keepalive")
			}
			if err := helper.WithRetry(ctx, operation); err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, "[ConsulDiscovery] Failed to update TTL")
				if err = d.checkAndReRegisterServices(ctx); err != nil {
					d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ConsulDiscovery] The re-registration service failed: %v", err))
				}
			}
			d.pool.Put(cli)
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeInfo, "[ConsulDiscovery] Stopping keepalive")
			return
		}
	}
}

// Deregister .
func (d *ConsulDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	operation := func() error {
		return cli.Agent().ServiceDeregister(instanceID)
	}
	if err := helper.WithRetry(ctx, operation); err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	d.mutex.Lock()
	delete(d.registeredServices, instanceID)
	d.mutex.Unlock()

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[ConsulDiscovery] Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *ConsulDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	ch := make(chan []base.ServiceInstance, 1)
	go func() {
		defer close(ch)
		lastIndex := uint64(0)
		for {
			cli := d.pool.Get()
			var services []*api.ServiceEntry
			var meta *api.QueryMeta

			operation := func() error {
				var queryErr error
				services, meta, queryErr = cli.Health().Service(serviceName, "", true, &api.QueryOptions{WaitIndex: lastIndex})
				return queryErr
			}
			if err := helper.WithRetry(ctx, operation); err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ConsulDiscovery] Failed to query services: %v", err))

				if err = d.checkAndReRegisterServices(ctx); err != nil {
					d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ConsulDiscovery] The re-registration service failed: %v", err))
				}
				d.pool.Put(cli)
				time.Sleep(time.Second)
				continue
			}

			lastIndex = meta.LastIndex
			instances := d.servicesToInstances(services)

			d.pool.Put(cli)

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
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	var services []*api.ServiceEntry
	operation := func() error {
		var queryErr error
		services, _, queryErr = cli.Health().Service(serviceName, "", true, nil)
		return queryErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		if err := d.checkAndReRegisterServices(context.Background()); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
		}

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
	d.pool.Close()
	return nil
}
