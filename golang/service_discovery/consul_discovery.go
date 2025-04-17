package service_discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

// ConsulDiscovery .
type ConsulDiscovery struct {
	client            *api.Client
	ttl               time.Duration
	keepAlive         time.Duration
	logOutputHookFunc LogOutputHookFunc
}

// NewConsulDiscovery .
func NewConsulDiscovery(addrs string, ttl, keepAlive time.Duration) (*ConsulDiscovery, error) {
	cfg := api.DefaultConfig()
	cfg.Address = strings.Split(addrs, ",")[0]
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Consul: %v", err)
	}

	return &ConsulDiscovery{
		client:    client,
		ttl:       ttl,
		keepAlive: keepAlive,
	}, nil
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
func (d *ConsulDiscovery) Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}
	host, port, err := splitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address: %v", err)
	}
	registration := &api.AgentServiceRegistration{
		ID:      instanceID,
		Name:    serviceName,
		Address: host,
		Port:    port,
		Meta: map[string]string{
			"http_port": httpPort,
			"grpc_port": grpcPort,
		},
		Check: &api.AgentServiceCheck{
			TTL:                            (d.ttl + time.Second).String(),
			DeregisterCriticalServiceAfter: (d.ttl * 2).String(),
		},
	}
	if err := d.client.Agent().ServiceRegister(registration); err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}
	go d.keepAliveLoop(ctx, instanceID)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, addr: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, addr, httpPort, grpcPort))
	return nil
}

// keepAliveLoop .
func (d *ConsulDiscovery) keepAliveLoop(ctx context.Context, instanceID string) {
	ticker := time.NewTicker(d.keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := d.client.Agent().PassTTL("service:"+instanceID, "keepalive")
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
	err := d.client.Agent().ServiceDeregister(instanceID)
	if err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *ConsulDiscovery) Watch(ctx context.Context, serviceName string) (chan []types.Instance, error) {
	ch := make(chan []types.Instance, 1)
	go func() {
		defer close(ch)
		lastIndex := uint64(0)
		for {
			services, meta, err := d.client.Health().Service(serviceName, "", true, &api.QueryOptions{WaitIndex: lastIndex})
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
func (d *ConsulDiscovery) GetInstances(serviceName string) ([]types.Instance, error) {
	services, _, err := d.client.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	return d.servicesToInstances(services), nil
}

// servicesToInstances .
func (d *ConsulDiscovery) servicesToInstances(services []*api.ServiceEntry) []types.Instance {
	var instances []types.Instance
	for _, svc := range services {
		instances = append(instances, types.Instance{
			InstanceID: svc.Service.ID,
			Addr:       fmt.Sprintf("%s:%d", svc.Service.Address, svc.Service.Port),
			Metadata:   svc.Service.Meta,
		})
	}
	return instances
}

// Close .
func (d *ConsulDiscovery) Close() error {
	return nil
}
