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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/helper"
)

// NacosDiscovery .
type NacosDiscovery struct {
	client            naming_client.INamingClient
	logOutputHookFunc LogOutputHookFunc
	clientConfig      NacosDiscoveryConfig
}

// NacosDiscoveryConfig .
type NacosDiscoveryConfig struct {
	TimeoutMs            uint64
	BeatInterval         int64
	NamespaceId          string
	AppName              string
	AppKey               string
	Endpoint             string
	RegionId             string
	AccessKey            string
	SecretKey            string
	OpenKMS              bool
	CacheDir             string
	DisableUseSnapShot   bool
	UpdateThreadNum      int
	NotLoadCacheAtStart  bool
	UpdateCacheWhenEmpty bool
	LogDir               string
	LogLevel             string
	ContextPath          string
	AppendToStdout       bool
	AsyncUpdateService   bool
	EndpointContextPath  string
	EndpointQueryParams  string
	ClusterName          string

	address        []string
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
	if clientConfig.maxRetries <= 0 {
		clientConfig.maxRetries = 3
	}

	if !helper.IsDurationSet(clientConfig.baseRetryDelay) {
		clientConfig.baseRetryDelay = 500 * time.Millisecond
	}

	namingClient, err := createNacosClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Nacos: %v", err)
	}

	return &NacosDiscovery{
		client:       namingClient,
		clientConfig: clientConfig,
	}, nil
}

// createNacosClient try to create a Nacos client that includes a retry mechanism
func createNacosClient(clientConfig NacosDiscoveryConfig) (naming_client.INamingClient, error) {
	var client naming_client.INamingClient
	var err error

	clientCfg := *constant.NewClientConfig(
		constant.WithNamespaceId(""),
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
	)

	if clientConfig.username != "" {
		clientCfg.Username = clientConfig.username
	}

	if clientConfig.password != "" {
		clientCfg.Password = clientConfig.password
	}

	if clientConfig.TimeoutMs > 0 {
		clientCfg.TimeoutMs = clientConfig.TimeoutMs
	}
	if clientConfig.BeatInterval > 0 {
		clientCfg.BeatInterval = clientConfig.BeatInterval
	}
	if clientConfig.NamespaceId != "" {
		clientCfg.NamespaceId = clientConfig.NamespaceId
	}
	if clientConfig.AppName != "" {
		clientCfg.AppName = clientConfig.AppName
	}
	if clientConfig.AppKey != "" {
		clientCfg.AppKey = clientConfig.AppKey
	}
	if clientConfig.AppKey != "" {
		clientCfg.AppKey = clientConfig.AppKey
	}
	if clientConfig.Endpoint != "" {
		clientCfg.Endpoint = clientConfig.Endpoint
	}
	if clientConfig.RegionId != "" {
		clientCfg.RegionId = clientConfig.RegionId
	}
	if clientConfig.AccessKey != "" {
		clientCfg.AccessKey = clientConfig.AccessKey
	}
	if clientConfig.SecretKey != "" {
		clientCfg.SecretKey = clientConfig.SecretKey
	}
	if clientConfig.OpenKMS {
		clientCfg.OpenKMS = clientConfig.OpenKMS
	}
	if clientConfig.CacheDir != "" {
		clientCfg.CacheDir = clientConfig.CacheDir
	}
	if clientConfig.DisableUseSnapShot {
		clientCfg.DisableUseSnapShot = clientConfig.DisableUseSnapShot
	}
	if clientConfig.UpdateThreadNum > 0 {
		clientCfg.UpdateThreadNum = clientConfig.UpdateThreadNum
	}
	if clientConfig.NotLoadCacheAtStart {
		clientCfg.NotLoadCacheAtStart = clientConfig.NotLoadCacheAtStart
	}
	if clientConfig.UpdateCacheWhenEmpty {
		clientCfg.UpdateCacheWhenEmpty = clientConfig.UpdateCacheWhenEmpty
	}
	if clientConfig.LogDir != "" {
		clientCfg.LogDir = clientConfig.LogDir
	}
	if clientConfig.LogLevel != "" {
		clientCfg.LogLevel = clientConfig.LogLevel
	}
	if clientConfig.ContextPath != "" {
		clientCfg.ContextPath = clientConfig.ContextPath
	}
	if clientConfig.AppendToStdout {
		clientCfg.AppendToStdout = clientConfig.AppendToStdout
	}
	if clientConfig.AsyncUpdateService {
		clientCfg.AsyncUpdateService = clientConfig.AsyncUpdateService
	}
	if clientConfig.EndpointContextPath != "" {
		clientCfg.EndpointContextPath = clientConfig.EndpointContextPath
	}
	if clientConfig.EndpointQueryParams != "" {
		clientCfg.EndpointQueryParams = clientConfig.EndpointQueryParams
	}
	if clientConfig.ClusterName != "" {
		clientCfg.ClusterName = clientConfig.ClusterName
	}

	if clientConfig.tlsConfig != nil {
		clientCfg.TLSCfg = constant.TLSConfig{
			Enable:             true,
			CaFile:             clientConfig.tlsConfig.CAFile,
			CertFile:           clientConfig.tlsConfig.CertFile,
			KeyFile:            clientConfig.tlsConfig.KeyFile,
			ServerNameOverride: clientConfig.tlsConfig.ServerName,
		}
	}

	var serverCfgs []constant.ServerConfig
	for _, addr := range clientConfig.address {
		hostPort := strings.Split(addr, ":")
		host := hostPort[0]
		port, _ := strconv.Atoi(hostPort[1])
		serverCfgs = append(serverCfgs, *constant.NewServerConfig(host, uint64(port)))
	}

	for attempt := 1; attempt <= clientConfig.maxRetries; attempt++ {
		client, err = clients.NewNamingClient(
			vo.NacosClientParam{
				ClientConfig:  &clientCfg,
				ServerConfigs: serverCfgs,
			},
		)
		if err == nil {
			return client, nil
		}

		d := 1 << uint(attempt-1)
		delay := time.Duration(float64(clientConfig.baseRetryDelay) * float64(d))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay + jitter)
	}

	return nil, fmt.Errorf("after %d attempts, it still couldn't connect to Nacos: %v", clientConfig.maxRetries, err)
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

// withRetry perform the operation using the retry logic
func (d *NacosDiscovery) withRetry(ctx context.Context, operation func() (bool, error)) (bool, error) {
	var success bool
	var err error
	for attempt := 1; attempt <= d.clientConfig.maxRetries; attempt++ {
		success, err = operation()
		if success && err == nil {
			return true, nil
		}

		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Operation failed. the %d/%d attempt: %v", attempt, d.clientConfig.maxRetries, err))

		dd := 1 << uint(attempt-1)
		delay := time.Duration(float64(d.clientConfig.baseRetryDelay) * float64(dd))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		select {
		case <-time.After(delay + jitter):
		case <-ctx.Done():
			return false, ctx.Err()
		}

		if attempt < d.clientConfig.maxRetries {
			newClient, clientErr := createNacosClient(d.clientConfig)
			if clientErr == nil {
				d.client = newClient
				d.outLog(ServiceDiscoveryLogTypeInfo, "Successfully reconnected to Nacos")
			}
		}
	}
	return success, fmt.Errorf("the operation failed after %d attempts: %v", d.clientConfig.maxRetries, err)
}

// Register .
func (d *NacosDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	port, _ := strconv.Atoi(httpPort)
	success, err := d.withRetry(ctx, func() (bool, error) {
		return d.client.RegisterInstance(vo.RegisterInstanceParam{
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
	})

	if !success || err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
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

	success, err := d.withRetry(ctx, func() (bool, error) {
		return d.client.DeregisterInstance(vo.DeregisterInstanceParam{
			Ip:          instance.Ip,
			Port:        instance.Port,
			ServiceName: serviceName,
			Ephemeral:   true,
		})
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
func (d *NacosDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	ch := make(chan []base.ServiceInstance, 1)
	go func() {
		defer close(ch)
		subscribeParam := &vo.SubscribeParam{
			ServiceName: serviceName,
			SubscribeCallback: func(services []model.Instance, err error) {
				if err != nil {
					d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Subscribe callback error: %v", err))
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

		_, err := d.withRetry(ctx, func() (bool, error) {
			subscribeErr := d.client.Subscribe(subscribeParam)
			return subscribeErr == nil, subscribeErr
		})
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Failed to subscribe: %v", err))
			return
		}
		<-ctx.Done()
		_ = d.client.Unsubscribe(subscribeParam)
	}()
	return ch, nil
}

// recoverSubscribe try to restore the subscription
func (d *NacosDiscovery) recoverSubscribe(ctx context.Context, serviceName string, ch chan []base.ServiceInstance) {
	subscribeParam := &vo.SubscribeParam{
		ServiceName: serviceName,
		SubscribeCallback: func(services []model.Instance, err error) {
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Subscribe to NACOS callback error: %v", err))
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

	_, err := d.withRetry(ctx, func() (bool, error) {
		subscribeErr := d.client.Subscribe(subscribeParam)
		return subscribeErr == nil, subscribeErr
	})
	if err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The NACOS subscription cannot be restored: %v", err))
		return
	}
	d.outLog(ServiceDiscoveryLogTypeInfo, "Successfully restored the NACOS subscription")
}

// GetInstances .
func (d *NacosDiscovery) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
	var service model.Service
	_, err := d.withRetry(context.Background(), func() (bool, error) {
		var getErr error
		service, getErr = d.client.GetService(vo.GetServiceParam{
			ServiceName: serviceName,
		})
		return getErr == nil, getErr
	})

	if err != nil {
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
	return nil
}
