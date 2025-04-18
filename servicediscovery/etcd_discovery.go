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
	"math/rand"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/wenlng/go-captcha-discovery/base"
	"github.com/wenlng/go-captcha-discovery/helper"
	"go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// EtcdDiscovery .
type EtcdDiscovery struct {
	client            *clientv3.Client
	leaseID           clientv3.LeaseID
	keepAliveCh       <-chan *clientv3.LeaseKeepAliveResponse
	logOutputHookFunc LogOutputHookFunc

	config EtcdDiscoveryConfig
}

// EtcdDiscoveryConfig .
type EtcdDiscoveryConfig struct {
	AutoSyncInterval      time.Duration
	DialTimeout           time.Duration
	DialKeepAliveTime     time.Duration
	DialKeepAliveTimeout  time.Duration
	MaxCallSendMsgSize    int
	MaxCallRecvMsgSize    int
	Username              string
	Password              string
	RejectOldCluster      bool
	DialOptions           []grpc.DialOption
	Context               context.Context
	Logger                *zap.Logger
	LogConfig             *zap.Config
	PermitWithoutStream   bool
	MaxUnaryRetries       uint
	BackoffWaitBetween    time.Duration
	BackoffJitterFraction float64

	address        []string
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
	if config.maxRetries <= 0 {
		config.maxRetries = 3
	}

	if !helper.IsDurationSet(config.baseRetryDelay) {
		config.baseRetryDelay = 500 * time.Millisecond
	}

	client, err := createEtcdClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %v", err)
	}

	return &EtcdDiscovery{
		client: client,
		config: config,
	}, nil
}

// createEtcdClient try to create a Etcd client that includes a retry mechanism
func createEtcdClient(config EtcdDiscoveryConfig) (*clientv3.Client, error) {
	cfg := clientv3.Config{
		Endpoints:   config.address,
		DialTimeout: 5 * time.Second,
	}

	if config.username != "" {
		cfg.Username = config.username
	}

	if config.password != "" {
		cfg.Password = config.password
	}

	if helper.IsDurationSet(config.AutoSyncInterval) {
		cfg.AutoSyncInterval = config.AutoSyncInterval
	}
	if helper.IsDurationSet(config.DialTimeout) {
		cfg.DialTimeout = config.DialTimeout
	}
	if helper.IsDurationSet(config.DialKeepAliveTime) {
		cfg.DialKeepAliveTime = config.DialKeepAliveTime
	}
	if helper.IsDurationSet(config.DialKeepAliveTimeout) {
		cfg.DialKeepAliveTimeout = config.DialKeepAliveTimeout
	}
	if config.MaxCallSendMsgSize > 0 {
		cfg.MaxCallSendMsgSize = config.MaxCallSendMsgSize
	}
	if config.MaxCallRecvMsgSize > 0 {
		cfg.MaxCallRecvMsgSize = config.MaxCallRecvMsgSize
	}

	if config.RejectOldCluster {
		cfg.RejectOldCluster = config.RejectOldCluster
	}
	if len(config.DialOptions) > 0 {
		cfg.DialOptions = config.DialOptions
	}
	if config.Context != nil {
		cfg.Context = config.Context
	}
	if config.Logger != nil {
		cfg.Logger = config.Logger
	}
	if config.LogConfig != nil {
		cfg.LogConfig = config.LogConfig
	}
	if config.PermitWithoutStream {
		cfg.PermitWithoutStream = config.PermitWithoutStream
	}
	if config.MaxUnaryRetries > 0 {
		cfg.MaxUnaryRetries = config.MaxUnaryRetries
	}
	if helper.IsDurationSet(config.BackoffWaitBetween) {
		cfg.BackoffWaitBetween = config.BackoffWaitBetween
	}
	if config.BackoffJitterFraction > 0.0 {
		cfg.BackoffJitterFraction = config.BackoffJitterFraction
	}

	if config.tlsConfig != nil {
		var err error
		cfg.TLS, err = base.CreateTLSConfig(config.tlsConfig)
		if err != nil {
			return nil, err
		}
	}

	var client *clientv3.Client
	var err error

	for attempt := 1; attempt <= config.maxRetries; attempt++ {
		client, err = clientv3.New(cfg)
		if err == nil {
			return client, nil
		}

		d := 1 << uint(attempt-1)
		delay := time.Duration(float64(config.baseRetryDelay) * float64(d))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay + jitter)
	}

	return nil, fmt.Errorf("after %d attempts, it still couldn't connect to Etcd: %v", config.maxRetries, err)
}

// SetLogOutputHookFunc .
func (d *EtcdDiscovery) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
	d.logOutputHookFunc = logOutputHookFunc
}

// outLog
func (d *EtcdDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.logOutputHookFunc != nil {
		d.logOutputHookFunc(logType, message)
	}
}

// withRetry perform the operation using the retry logic
func (d *EtcdDiscovery) withRetry(ctx context.Context, operation func() error) error {
	var err error
	for attempt := 1; attempt <= d.config.maxRetries; attempt++ {
		err = operation()
		if err == nil {
			return nil
		}

		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Operation failed. the %d/%d attempt: %v", attempt, d.config.maxRetries, err))

		dd := 1 << uint(attempt-1)
		delay := time.Duration(float64(d.config.baseRetryDelay) * float64(dd))
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		select {
		case <-time.After(delay + jitter):
		case <-ctx.Done():
			return ctx.Err()
		}

		if attempt < d.config.maxRetries {
			newClient, clientErr := createEtcdClient(d.config)
			if clientErr == nil {
				d.client.Close()
				d.client = newClient
				d.outLog(ServiceDiscoveryLogTypeInfo, "Successfully reconnected to Etcd")
			}
		}
	}
	return fmt.Errorf("the operation failed after %d attempts: %v", d.config.maxRetries, err)
}

// Register .
func (d *EtcdDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	var leaseResp *clientv3.LeaseGrantResponse
	err := d.withRetry(ctx, func() error {
		var grantErr error
		leaseResp, grantErr = d.client.Grant(ctx, int64(d.config.ttl/time.Second))
		return grantErr
	})
	if err != nil {
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
	err = d.withRetry(ctx, func() error {
		_, putErr := d.client.Put(ctx, key, string(data), clientv3.WithLease(d.leaseID))
		return putErr
	})
	if err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	err = d.withRetry(ctx, func() error {
		var keepAliveErr error
		d.keepAliveCh, keepAliveErr = d.client.KeepAlive(ctx, d.leaseID)
		return keepAliveErr
	})
	if err != nil {
		return fmt.Errorf("failed to start keepalive: %v", err)
	}

	go d.watchKeepAlive(ctx)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
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
	err := d.withRetry(ctx, func() error {
		var keepAliveErr error
		d.keepAliveCh, keepAliveErr = d.client.KeepAlive(ctx, d.leaseID)
		return keepAliveErr
	})
	if err != nil {
		d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to restore the Etcd heartbeat: %v", err))
		return
	}
	d.outLog(ServiceDiscoveryLogTypeInfo, "The heart rate of Ectd was successfully restored")
	go d.watchKeepAlive(ctx)
}

// Deregister .
func (d *EtcdDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	key := path.Join("/services", serviceName, instanceID)
	err := d.withRetry(ctx, func() error {
		_, deleteErr := d.client.Delete(ctx, key)
		return deleteErr
	})
	if err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}
	if d.leaseID != 0 {
		err = d.withRetry(ctx, func() error {
			_, revokeErr := d.client.Revoke(ctx, d.leaseID)
			return revokeErr
		})
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to revoke lease: %v", err))
		}
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *EtcdDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	prefix := path.Join("/services", serviceName)
	ch := make(chan []base.ServiceInstance, 1)
	rch := d.client.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(ch)
		for resp := range rch {
			instances, err := d.getInstancesFromEvents(serviceName, resp.Events)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to parse watch events: %v", err))
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
	prefix := path.Join("/services", serviceName)
	rch := d.client.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		for resp := range rch {
			if resp.Err() != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Monitor Etcd event errors: %v", resp.Err()))
				continue
			}
			instances, err := d.getInstancesFromEvents(serviceName, resp.Events)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The Etcd monitoring event cannot be parsed: %v", err))
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
	prefix := path.Join("/services", serviceName)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancel()

	var resp *clientv3.GetResponse
	err := d.withRetry(ctx, func() error {
		var getErr error
		resp, getErr = d.client.Get(ctx, prefix, clientv3.WithPrefix())
		return getErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	var instances []base.ServiceInstance
	for _, kv := range resp.Kvs {
		var inst base.ServiceInstance
		if err = json.Unmarshal(kv.Value, &inst); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to unmarshal instance: %v", err))
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
	if d.leaseID != 0 {
		_, err := d.client.Revoke(context.Background(), d.leaseID)
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to revoke lease on close: %v", err))
		}
	}
	return d.client.Close()
}
