package service_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
	"go.etcd.io/etcd/client/v3"
)

// EtcdDiscovery .
type EtcdDiscovery struct {
	client            *clientv3.Client
	ttl               time.Duration
	keepAlive         time.Duration
	leaseID           clientv3.LeaseID
	keepAliveCh       <-chan *clientv3.LeaseKeepAliveResponse
	logOutputHookFunc LogOutputHookFunc
}

// NewEtcdDiscovery .
func NewEtcdDiscovery(addrs string, ttl, keepAlive time.Duration) (*EtcdDiscovery, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(addrs, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %v", err)
	}
	return &EtcdDiscovery{
		client:    client,
		ttl:       ttl,
		keepAlive: keepAlive,
	}, nil
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

// Register .
func (d *EtcdDiscovery) Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}
	leaseResp, err := d.client.Grant(ctx, int64(d.ttl/time.Second))
	if err != nil {
		return fmt.Errorf("failed to grant lease: %v", err)
	}
	d.leaseID = leaseResp.ID

	data, err := json.Marshal(types.Instance{
		InstanceID: instanceID,
		Addr:       addr,
		Metadata: map[string]string{
			"http_port": httpPort,
			"grpc_port": grpcPort,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal instance: %v", err)
	}

	key := path.Join("/services", serviceName, instanceID)
	_, err = d.client.Put(ctx, key, string(data), clientv3.WithLease(d.leaseID))
	if err != nil {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	d.keepAliveCh, err = d.client.KeepAlive(ctx, d.leaseID)
	if err != nil {
		return fmt.Errorf("failed to start keepalive: %v", err)
	}
	go d.watchKeepAlive(ctx)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, addr: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, addr, httpPort, grpcPort))
	return nil
}

// watchKeepAlive .
func (d *EtcdDiscovery) watchKeepAlive(ctx context.Context) {
	for {
		select {
		case _, ok := <-d.keepAliveCh:
			if !ok {
				d.outLog(ServiceDiscoveryLogTypeWarn, "KeepAlive channel closed")
				return
			}
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeWarn, "Stopping keepalive")
			return
		}
	}
}

// Deregister .
func (d *EtcdDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	key := path.Join("/services", serviceName, instanceID)
	_, err := d.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}
	if d.leaseID != 0 {
		_, err = d.client.Revoke(ctx, d.leaseID)
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
func (d *EtcdDiscovery) Watch(ctx context.Context, serviceName string) (chan []types.Instance, error) {
	prefix := path.Join("/services", serviceName)
	ch := make(chan []types.Instance, 1)
	rch := d.client.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(ch)
		for resp := range rch {
			instances, err := d.getInstancesFromEvents(serviceName, resp.Events)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to parse watch events: %v", err))
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

// GetInstances .
func (d *EtcdDiscovery) GetInstances(serviceName string) ([]types.Instance, error) {
	prefix := path.Join("/services", serviceName)
	resp, err := d.client.Get(context.Background(), prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	var instances []types.Instance
	for _, kv := range resp.Kvs {
		var inst types.Instance
		if err = json.Unmarshal(kv.Value, &inst); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to unmarshal instance: %v", err))
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// getInstancesFromEvents .
func (d *EtcdDiscovery) getInstancesFromEvents(serviceName string, events []*clientv3.Event) ([]types.Instance, error) {
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
