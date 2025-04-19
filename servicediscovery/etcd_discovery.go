/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
	"go.etcd.io/etcd/client/v3"
)

// EtcdDiscovery .
type EtcdDiscovery struct {
	//client            *clientv3.Client
	leaseID           clientv3.LeaseID
	keepAliveCh       <-chan *clientv3.LeaseKeepAliveResponse
	pool              *clientpool.EtcdPool
	outputLogCallback OutputLogCallback
	config            EtcdDiscoveryConfig

	registeredServices map[string]registeredServiceInfo
	mutex              sync.RWMutex
}

// EtcdDiscoveryConfig .
type EtcdDiscoveryConfig struct {
	extraconfig.EtcdExtraConfig

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

// NewEtcdDiscovery .
func NewEtcdDiscovery(config EtcdDiscoveryConfig) (*EtcdDiscovery, error) {
	if config.poolSize <= 0 {
		config.poolSize = 5
	}

	if config.maxRetries <= 0 {
		config.maxRetries = 3
	}

	if !helper.IsDurationSet(config.baseRetryDelay) {
		config.baseRetryDelay = 500 * time.Millisecond
	}

	pool, err := clientpool.NewEtcdPool(config.poolSize, config.address, &config.EtcdExtraConfig)
	if err != nil {
		return nil, err
	}

	return &EtcdDiscovery{
		config:             config,
		pool:               pool,
		registeredServices: make(map[string]registeredServiceInfo),
	}, nil
}

// SetOutputLogCallback .
func (d *EtcdDiscovery) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
	d.outputLogCallback = outputLogCallback
}

// outLog
func (d *EtcdDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.outputLogCallback != nil {
		d.outputLogCallback(logType, message)
	}
}

// checkAndReRegisterServices check and re-register the service
func (d *EtcdDiscovery) checkAndReRegisterServices(ctx context.Context) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	d.mutex.RLock()
	services := make(map[string]registeredServiceInfo)
	for k, v := range d.registeredServices {
		services[k] = v
	}
	d.mutex.RUnlock()

	for instanceID, svcInfo := range services {
		key := path.Join("/services", svcInfo.ServiceName, instanceID)
		operation := func() error {
			resp, err := cli.Get(ctx, key)
			if err != nil {
				return fmt.Errorf("failed to check the service registration status: %v", err)
			}
			if len(resp.Kvs) == 0 {
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
func (d *EtcdDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	var leaseResp *clientv3.LeaseGrantResponse
	operation := func() error {
		var grantErr error
		leaseResp, grantErr = cli.Grant(ctx, int64(d.config.ttl/time.Second))
		return grantErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		return fmt.Errorf("failed to grant lease: %v", err)
	}

	d.leaseID = leaseResp.ID
	data, err := json.Marshal(base.ServiceInstance{
		InstanceID: instanceID,
		Host:       host,
		HTTPPort:   httpPort,
		GRPCPort:   grpcPort,
		Metadata: map[string]string{
			"hostname":  helper.GetHostname(),
			"host":      host,
			"http_port": httpPort,
			"grpc_port": grpcPort,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal instance: %v", err)
	}

	key := path.Join("/services", serviceName, instanceID)
	operation = func() error {
		_, putErr := cli.Put(ctx, key, string(data), clientv3.WithLease(d.leaseID))
		return putErr
	}
	if err = helper.WithRetry(context.Background(), operation); err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	operation = func() error {
		var keepAliveErr error
		d.keepAliveCh, keepAliveErr = cli.KeepAlive(ctx, d.leaseID)
		return keepAliveErr
	}
	if err = helper.WithRetry(context.Background(), operation); err != nil {
		return fmt.Errorf("failed to start keepalive: %v", err)
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

	go d.watchKeepAlive(ctx)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[EtcdDiscovery] Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
	return nil
}

// watchKeepAlive .
func (d *EtcdDiscovery) watchKeepAlive(ctx context.Context) {
	for {
		select {
		case _, ok := <-d.keepAliveCh:
			if !ok {
				d.outLog(ServiceDiscoveryLogTypeWarn, "KeepAlive channel closed")
				go d.recoverKeepAlive(ctx)
				return
			}
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeWarn, "Stopping keepalive")
			return
		}
	}
}

// recoverKeepAlive try to restore the heartbeat
func (d *EtcdDiscovery) recoverKeepAlive(ctx context.Context) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	operation := func() error {
		var keepAliveErr error
		d.keepAliveCh, keepAliveErr = cli.KeepAlive(ctx, d.leaseID)
		return keepAliveErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] Failed to restore the Etcd heartbeat: %v", err))
		return
	}

	if err := d.checkAndReRegisterServices(ctx); err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
	}

	go d.watchKeepAlive(ctx)
	d.outLog(ServiceDiscoveryLogTypeInfo, "The heart rate of Ectd was successfully restored")
}

// Deregister .
func (d *EtcdDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	key := path.Join("/services", serviceName, instanceID)
	operation := func() error {
		_, deleteErr := cli.Delete(ctx, key)
		return deleteErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	if d.leaseID != 0 {
		operation = func() error {
			_, revokeErr := cli.Revoke(ctx, d.leaseID)
			return revokeErr
		}
		if err := helper.WithRetry(context.Background(), operation); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] Failed to revoke lease: %v", err))
		}
	}

	d.mutex.Lock()
	delete(d.registeredServices, instanceID)
	d.mutex.Unlock()

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[EtcdDiscovery] Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *EtcdDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	prefix := path.Join("/services", serviceName)
	ch := make(chan []base.ServiceInstance, 1)
	rch := cli.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(ch)
		for resp := range rch {
			instances, err := d.getInstancesFromEvents(serviceName, resp.Events)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] Failed to parse watch events: %v", err))
				go d.recoverWatch(ctx, serviceName, ch)
				continue
			}

			select {
			case ch <- instances:
			case <-ctx.Done():
				return
			}
		}
	}()
	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return nil, err
	}
	if len(instances) > 0 {
		ch <- instances
	}
	return ch, nil
}

// recoverWatch attempt to restore surveillance
func (d *EtcdDiscovery) recoverWatch(ctx context.Context, serviceName string, ch chan []base.ServiceInstance) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	prefix := path.Join("/services", serviceName)
	rch := cli.Watch(ctx, prefix, clientv3.WithPrefix())

	if err := d.checkAndReRegisterServices(ctx); err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
	}

	go func() {
		for resp := range rch {
			if resp.Err() != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] Monitor Etcd event errors: %v", resp.Err()))
				continue
			}
			instances, err := d.getInstancesFromEvents(serviceName, resp.Events)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] The Etcd monitoring event cannot be parsed: %v", err))
				continue
			}

			select {
			case ch <- instances:
			case <-ctx.Done():
				return
			}
		}
	}()

	d.outLog(ServiceDiscoveryLogTypeInfo, "The ETCD monitoring was successfully restored")
}

// GetInstances .
func (d *EtcdDiscovery) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	prefix := path.Join("/services", serviceName)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancel()

	var resp *clientv3.GetResponse
	operation := func() error {
		var getErr error
		resp, getErr = cli.Get(ctx, prefix, clientv3.WithPrefix())
		return getErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		if err = d.checkAndReRegisterServices(ctx); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
		}
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}

	var instances []base.ServiceInstance
	for _, kv := range resp.Kvs {
		var inst base.ServiceInstance
		if err := json.Unmarshal(kv.Value, &inst); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] Failed to unmarshal instance: %v", err))
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// getInstancesFromEvents .
func (d *EtcdDiscovery) getInstancesFromEvents(serviceName string, events []*clientv3.Event) ([]base.ServiceInstance, error) {
	instances, err := d.GetInstances(serviceName)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

// Close .
func (d *EtcdDiscovery) Close() error {
	cli := d.pool.Get()

	if d.leaseID != 0 {
		_, err := cli.Revoke(context.Background(), d.leaseID)
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[EtcdDiscovery] Failed to revoke lease on close: %v", err))
		}
	}

	d.pool.Close()
	return nil
}
