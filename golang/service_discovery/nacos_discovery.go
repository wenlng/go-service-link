package service_discovery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

// NacosDiscovery .
type NacosDiscovery struct {
	client            naming_client.INamingClient
	ttl               time.Duration
	keepAlive         time.Duration
	logOutputHookFunc LogOutputHookFunc
}

// NewNacosDiscovery .
func NewNacosDiscovery(addrs string, ttl, keepAlive time.Duration) (*NacosDiscovery, error) {
	clientConfig := *constant.NewClientConfig(
		constant.WithNamespaceId(""),
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
	)
	var serverConfigs []constant.ServerConfig
	for _, addr := range strings.Split(addrs, ",") {
		hostPort := strings.Split(addr, ":")
		host := hostPort[0]
		port, _ := strconv.Atoi(hostPort[1])
		serverConfigs = append(serverConfigs, *constant.NewServerConfig(host, uint64(port)))
	}
	namingClient, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Nacos: %v", err)
	}

	return &NacosDiscovery{
		client:    namingClient,
		ttl:       ttl,
		keepAlive: keepAlive,
	}, nil
}

// SetLogOutputHookFunc .
func (d *NacosDiscovery) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
	d.logOutputHookFunc = logOutputHookFunc
}

// outLog
func (d *NacosDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.logOutputHookFunc != nil {
		d.logOutputHookFunc(logType, message)
	}
}

// Register .
func (d *NacosDiscovery) Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}
	host, port, err := splitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address: %v", err)
	}
	success, err := d.client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          host,
		Port:        uint64(port),
		ServiceName: serviceName,
		Weight:      1,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
		Metadata: map[string]string{
			"http_port":   httpPort,
			"grpc_port":   grpcPort,
			"instance_id": instanceID,
		},
	})

	if !success || err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get instances after register: %v", err)
	}

	for _, inst := range instances {
		if inst.Metadata["instance_id"] == instanceID && inst.Addr == addr {
			d.outLog(
				ServiceDiscoveryLogTypeInfo,
				fmt.Sprintf("Registered instance, service: %s, instanceId: %s, addr: %s, http_port: %s, grpc_port: %s",
					serviceName, instanceID, addr, httpPort, grpcPort))
			return nil
		}
	}

	return nil
}

// Deregister .
func (d *NacosDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get instances for deregister: %v", err)
	}
	var instance model.Instance
	for _, inst := range instances {
		if inst.InstanceID == instanceID {
			host, port, err := splitHostPort(inst.Addr)
			if err != nil {
				continue
			}
			instance = model.Instance{
				InstanceId:  instanceID,
				Ip:          host,
				Port:        uint64(port),
				ServiceName: serviceName,
			}
			break
		}
	}
	success, err := d.client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          instance.Ip,
		Port:        instance.Port,
		ServiceName: serviceName,
		Ephemeral:   true,
	})
	if !success || err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *NacosDiscovery) Watch(ctx context.Context, serviceName string) (chan []types.Instance, error) {
	ch := make(chan []types.Instance, 1)
	go func() {
		defer close(ch)
		subscribeParam := &vo.SubscribeParam{
			ServiceName: serviceName,
			SubscribeCallback: func(services []model.Instance, err error) {
				if err != nil {
					d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Subscribe callback error: %v", err))
					return
				}
				instances := make([]types.Instance, len(services))
				for i, svc := range services {
					instances[i] = types.Instance{
						InstanceID: svc.InstanceId,
						Addr:       fmt.Sprintf("%s:%d", svc.Ip, svc.Port),
						Metadata:   svc.Metadata,
					}
				}
				select {
				case ch <- instances:
				case <-ctx.Done():
					return
				}
			},
		}
		err := d.client.Subscribe(subscribeParam)
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Failed to subscribe: %v", err))
			return
		}
		<-ctx.Done()
		_ = d.client.Unsubscribe(subscribeParam)
	}()
	return ch, nil
}

// GetInstances .
func (d *NacosDiscovery) GetInstances(serviceName string) ([]types.Instance, error) {
	service, err := d.client.GetService(vo.GetServiceParam{
		ServiceName: serviceName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	result := make([]types.Instance, len(service.Hosts))
	for i, inst := range service.Hosts {
		result[i] = types.Instance{
			InstanceID: inst.InstanceId,
			Addr:       fmt.Sprintf("%s:%d", inst.Ip, inst.Port),
			Metadata:   inst.Metadata,
		}
	}
	return result, nil
}

// Close .
func (d *NacosDiscovery) Close() error {
	return nil
}
