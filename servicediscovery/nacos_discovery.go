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
	"github.com/wenlng/go-service-link/foundation/clientpool"
	"github.com/wenlng/go-service-link/foundation/common"
	"github.com/wenlng/go-service-link/foundation/extraconfig"
	"github.com/wenlng/go-service-link/foundation/helper"
	"github.com/wenlng/go-service-link/servicediscovery/instance"
)

// NacosDiscovery .
type NacosDiscovery struct {
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
	tlsConfig      *common.TLSConfig
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

	clientConfig.NacosExtraConfig.Username = clientConfig.username
	clientConfig.NacosExtraConfig.Password = clientConfig.password

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
func (d *NacosDiscovery) outLog(logType OutputLogType, message string) {
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
				d.outLog(OutputLogTypeWarn, fmt.Sprintf("The service has not been registered. Re-register: %s, instanceID: %s", svcInfo.ServiceName, instanceID))
				return d.register(ctx, svcInfo.ServiceName, instanceID, svcInfo.Host, svcInfo.HTTPPort, svcInfo.GRPCPort, true)
			}
			return nil
		}

		if err := helper.WithRetry(ctx, operation); err != nil {
			d.outLog(OutputLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
			return err
		}
	}
	return nil
}

// register .
func (d *NacosDiscovery) register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string, isReRegister bool) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	if !isReRegister {
		d.mutex.Lock()
		d.registeredServices[instanceID] = registeredServiceInfo{
			ServiceName: serviceName,
			InstanceID:  instanceID,
			Host:        host,
			HTTPPort:    httpPort,
			GRPCPort:    grpcPort,
		}
		d.mutex.Unlock()
	}

	d.outLog(
		OutputLogTypeInfo,
		fmt.Sprintf("[ConsulDiscovery] The registration service is beginning...., service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))

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
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		if err = d.checkAndReRegisterServices(ctx); err != nil {
			d.outLog(OutputLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
		}
	}

	if !success {
		return fmt.Errorf("RegisterInstance failed with success=false, serviceName: %s", serviceName)
	}

	d.outLog(
		OutputLogTypeInfo,
		fmt.Sprintf("[NacosDiscovery] Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
	return nil
}

// Register .
func (d *NacosDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	return d.register(ctx, serviceName, instanceID, host, httpPort, grpcPort, false)
}

// Deregister .
func (d *NacosDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get instances for deregister: %v", err)
	}
	var curInst model.Instance
	for _, inst := range instances {
		if inst.InstanceID == instanceID {
			port, _ := strconv.Atoi(inst.HTTPPort)
			curInst = model.Instance{
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
			Ip:          curInst.Ip,
			Port:        curInst.Port,
			ServiceName: serviceName,
			Ephemeral:   true,
		})
		return err
	}
	if err = helper.WithRetry(context.Background(), operation); !success || err != nil {
		return fmt.Errorf("failed to deregister curInst: %v", err)
	}

	d.mutex.Lock()
	delete(d.registeredServices, instanceID)
	d.mutex.Unlock()

	d.outLog(
		OutputLogTypeInfo,
		fmt.Sprintf("[NacosDiscovery] Deregistered curInst, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *NacosDiscovery) Watch(ctx context.Context, serviceName string) (chan []instance.ServiceInstance, error) {
	ch := make(chan []instance.ServiceInstance, 1)
	go func() {
		defer close(ch)

		subscribeParam := &vo.SubscribeParam{
			ServiceName: serviceName,
			SubscribeCallback: func(services []model.Instance, err error) {
				if err != nil {
					d.outLog(OutputLogTypeError, fmt.Sprintf("[NacosDiscovery] Subscribe callback error: %v", err))
					go d.recoverSubscribe(ctx, serviceName, ch)
					return
				}
				instances := make([]instance.ServiceInstance, len(services))
				for i, svc := range services {
					httpPort := strconv.FormatUint(svc.Port, 10)
					grpcPort := ""
					if port, ok := svc.Metadata["http_port"]; ok {
						httpPort = port
					}
					if port, ok := svc.Metadata["grpc_port"]; ok {
						grpcPort = port
					}

					instances[i] = instance.ServiceInstance{
						InstanceID: svc.InstanceId,
						Host:       svc.Ip,
						HTTPPort:   httpPort,
						GRPCPort:   grpcPort,
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
			d.outLog(OutputLogTypeError, fmt.Sprintf("[NacosDiscovery] Failed to subscribe: %v", err))
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
func (d *NacosDiscovery) recoverSubscribe(ctx context.Context, serviceName string, ch chan []instance.ServiceInstance) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	subscribeParam := &vo.SubscribeParam{
		ServiceName: serviceName,
		SubscribeCallback: func(services []model.Instance, err error) {
			if err != nil {
				d.outLog(OutputLogTypeError, fmt.Sprintf("[NacosDiscovery] Subscribe to NACOS callback error: %v", err))
				return
			}
			instances := make([]instance.ServiceInstance, len(services))
			for i, svc := range services {
				httpPort := strconv.FormatUint(svc.Port, 10)
				grpcPort := ""
				if port, ok := svc.Metadata["http_port"]; ok {
					httpPort = port
				}
				if port, ok := svc.Metadata["grpc_port"]; ok {
					grpcPort = port
				}

				instances[i] = instance.ServiceInstance{
					InstanceID: svc.InstanceId,
					Host:       svc.Ip,
					HTTPPort:   httpPort,
					GRPCPort:   grpcPort,
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
		d.outLog(OutputLogTypeWarn, fmt.Sprintf("[NacosDiscovery] The NACOS subscription cannot be restored: %v", err))
		return
	}

	d.outLog(OutputLogTypeInfo, "[NacosDiscovery] Successfully restored the NACOS subscription")

	if err := d.checkAndReRegisterServices(ctx); err != nil {
		d.outLog(OutputLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
	}
}

// GetInstances .
func (d *NacosDiscovery) GetInstances(serviceName string) ([]instance.ServiceInstance, error) {
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
		if err = d.checkAndReRegisterServices(context.Background()); err != nil {
			d.outLog(OutputLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
		}
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}

	result := make([]instance.ServiceInstance, len(service.Hosts))
	for i, inst := range service.Hosts {
		httpPort := strconv.FormatUint(inst.Port, 10)
		grpcPort := ""
		if port, ok := inst.Metadata["http_port"]; ok {
			httpPort = port
		}
		if port, ok := inst.Metadata["grpc_port"]; ok {
			grpcPort = port
		}

		result[i] = instance.ServiceInstance{
			InstanceID: inst.InstanceId,
			Host:       inst.Ip,
			Metadata:   inst.Metadata,
			HTTPPort:   httpPort,
			GRPCPort:   grpcPort,
		}
	}
	return result, nil
}

// Close .
func (d *NacosDiscovery) Close() error {
	d.pool.Close()
	return nil
}
