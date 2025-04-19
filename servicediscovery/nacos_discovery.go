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
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
)

// NacosDiscovery .
type NacosDiscovery struct {
	//client            naming_client.INamingClient
	outputLogCallback OutputLogCallback
	pool              *clientpool.NacosNamingPool
	clientConfig      NacosDiscoveryConfig

	registeredServices map[string]registeredServiceInfo
	mutex              sync.RWMutex
}

// NacosDiscoveryConfig .
type NacosDiscoveryConfig struct {
	extraconfig.NacosExtraConfig

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

// NewNacosDiscovery .
func NewNacosDiscovery(clientConfig NacosDiscoveryConfig) (*NacosDiscovery, error) {
	if clientConfig.poolSize <= 0 {
		clientConfig.poolSize = 5
	}

	if clientConfig.maxRetries <= 0 {
		clientConfig.maxRetries = 3
	}

	if !helper.IsDurationSet(clientConfig.baseRetryDelay) {
		clientConfig.baseRetryDelay = 500 * time.Millisecond
	}

	pool, err := clientpool.NewNacosNamingPool(clientConfig.poolSize, clientConfig.address, &clientConfig.NacosExtraConfig)
	if err != nil {
		return nil, err
	}

	return &NacosDiscovery{
		clientConfig:       clientConfig,
		pool:               pool,
		registeredServices: make(map[string]registeredServiceInfo),
	}, nil
}

// SetOutputLogCallback .
func (d *NacosDiscovery) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
	d.outputLogCallback = outputLogCallback
}

// outLog
func (d *NacosDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.outputLogCallback != nil {
		d.outputLogCallback(logType, message)
	}
}

// checkAndReRegisterServices check and re-register the service
func (d *NacosDiscovery) checkAndReRegisterServices(ctx context.Context) error {
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
			service, err := cli.GetService(vo.GetServiceParam{
				ServiceName: svcInfo.ServiceName,
			})
			if err != nil {
				return fmt.Errorf("failed to check the service registration status: %v", err)
			}

			registered := false
			for _, inst := range service.Hosts {
				if inst.InstanceId == instanceID {
					registered = true
					break
				}
			}

			if !registered {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The service has not been registered. Re-register: %s, instanceID: %s", svcInfo.ServiceName, instanceID))
				return d.Register(ctx, svcInfo.ServiceName, instanceID, svcInfo.Host, svcInfo.HTTPPort, svcInfo.GRPCPort)
			}
			return nil
		}

		if err := helper.WithRetry(ctx, operation); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
			return err
		}
	}
	return nil
}

// Register .
func (d *NacosDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	if instanceID == "" {

		instanceID = uuid.New().String()
	}

	port, _ := strconv.Atoi(httpPort)
	var success bool
	operation := func() error {
		var err error
		success, err = cli.RegisterInstance(vo.RegisterInstanceParam{
			Ip:          host,
			Port:        uint64(port),
			ServiceName: serviceName,
			Weight:      1,
			Enable:      true,
			Healthy:     true,
			Ephemeral:   true,
			Metadata: map[string]string{
				"hostname":    helper.GetHostname(),
				"host":        host,
				"http_port":   httpPort,
				"grpc_port":   grpcPort,
				"instance_id": instanceID,
			},
		})
		return err
	}
	if err := helper.WithRetry(context.Background(), operation); !success || err != nil {
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

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[NacosDiscovery] Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
	return nil
}

// Deregister .
func (d *NacosDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get instances for deregister: %v", err)
	}
	var instance model.Instance
	for _, inst := range instances {
		if inst.InstanceID == instanceID {
			port, _ := strconv.Atoi(inst.HTTPPort)
			instance = model.Instance{
				InstanceId:  instanceID,
				Ip:          inst.Host,
				Port:        uint64(port),
				ServiceName: serviceName,
			}
			break
		}
	}

	var success bool
	operation := func() error {
		success, err = cli.DeregisterInstance(vo.DeregisterInstanceParam{
			Ip:          instance.Ip,
			Port:        instance.Port,
			ServiceName: serviceName,
			Ephemeral:   true,
		})
		return err
	}
	if err = helper.WithRetry(context.Background(), operation); !success || err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	d.mutex.Lock()
	delete(d.registeredServices, instanceID)
	d.mutex.Unlock()

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[NacosDiscovery] Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *NacosDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	ch := make(chan []base.ServiceInstance, 1)
	go func() {
		defer close(ch)

		subscribeParam := &vo.SubscribeParam{
			ServiceName: serviceName,
			SubscribeCallback: func(services []model.Instance, err error) {
				if err != nil {
					d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("[NacosDiscovery] Subscribe callback error: %v", err))
					go d.recoverSubscribe(ctx, serviceName, ch)
					return
				}
				instances := make([]base.ServiceInstance, len(services))
				for i, svc := range services {
					instances[i] = base.ServiceInstance{
						InstanceID: svc.InstanceId,
						Host:       fmt.Sprintf("%s:%d", svc.Ip, svc.Port),
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

		cli := d.pool.Get()
		operation := func() error {
			subscribeErr := cli.Subscribe(subscribeParam)
			return subscribeErr
		}
		if err := helper.WithRetry(context.Background(), operation); err != nil {
			d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("[NacosDiscovery] Failed to subscribe: %v", err))
			d.pool.Put(cli)
			return
		}
		d.pool.Put(cli)

		<-ctx.Done()
		_ = cli.Unsubscribe(subscribeParam)
	}()
	return ch, nil
}

// recoverSubscribe try to restore the subscription
func (d *NacosDiscovery) recoverSubscribe(ctx context.Context, serviceName string, ch chan []base.ServiceInstance) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	subscribeParam := &vo.SubscribeParam{
		ServiceName: serviceName,
		SubscribeCallback: func(services []model.Instance, err error) {
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("[NacosDiscovery] Subscribe to NACOS callback error: %v", err))
				return
			}
			instances := make([]base.ServiceInstance, len(services))
			for i, svc := range services {
				instances[i] = base.ServiceInstance{
					InstanceID: svc.InstanceId,
					Host:       fmt.Sprintf("%s:%d", svc.Ip, svc.Port),
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

	operation := func() error {
		subscribeErr := cli.Subscribe(subscribeParam)
		return subscribeErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[NacosDiscovery] The NACOS subscription cannot be restored: %v", err))
		return
	}

	d.outLog(ServiceDiscoveryLogTypeInfo, "[NacosDiscovery] Successfully restored the NACOS subscription")

	if err := d.checkAndReRegisterServices(ctx); err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
	}
}

// GetInstances .
func (d *NacosDiscovery) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	var service model.Service

	operation := func() error {
		var getErr error
		service, getErr = cli.GetService(vo.GetServiceParam{
			ServiceName: serviceName,
		})
		return getErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		if err := d.checkAndReRegisterServices(context.Background()); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
		}
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}

	result := make([]base.ServiceInstance, len(service.Hosts))
	for i, inst := range service.Hosts {
		result[i] = base.ServiceInstance{
			InstanceID: inst.InstanceId,
			Host:       fmt.Sprintf("%s:%d", inst.Ip, inst.Port),
			Metadata:   inst.Metadata,
		}
	}
	return result, nil
}

// Close .
func (d *NacosDiscovery) Close() error {
	d.pool.Close()
	return nil
}
