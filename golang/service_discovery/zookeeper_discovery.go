package service_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/google/uuid"
	"github.com/wenlng/service-discovery/golang/service_discovery/types"
)

// ZooKeeperDiscovery .
type ZooKeeperDiscovery struct {
	conn              *zk.Conn
	ttl               time.Duration
	keepAlive         time.Duration
	instanceID        string
	logOutputHookFunc LogOutputHookFunc
}

// NewZooKeeperDiscovery .
func NewZooKeeperDiscovery(addrs string, ttl, keepAlive time.Duration) (*ZooKeeperDiscovery, error) {
	conn, _, err := zk.Connect(strings.Split(addrs, ","), time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ZooKeeper: %v", err)
	}

	return &ZooKeeperDiscovery{
		conn:      conn,
		ttl:       ttl,
		keepAlive: keepAlive,
	}, nil
}

// SetLogOutputHookFunc .
func (d *ZooKeeperDiscovery) SetLogOutputHookFunc(logOutputHookFunc LogOutputHookFunc) {
	d.logOutputHookFunc = logOutputHookFunc
}

// outLog
func (d *ZooKeeperDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.logOutputHookFunc != nil {
		d.logOutputHookFunc(logType, message)
	}
}

// Register .
func (d *ZooKeeperDiscovery) Register(ctx context.Context, serviceName, instanceID, addr, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}
	d.instanceID = instanceID

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

	p := path.Join("/services", serviceName, instanceID)
	_, err = d.conn.Create(p, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		return fmt.Errorf("failed to register instance: %v", err)
	}

	go d.keepAliveLoop(ctx, p, data)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, addr: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, addr, httpPort, grpcPort))
	return nil
}

// keepAliveLoop .
func (d *ZooKeeperDiscovery) keepAliveLoop(ctx context.Context, path string, data []byte) {
	ticker := time.NewTicker(d.keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			exists, _, err := d.conn.Exists(path)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to check instance: %v", err))
				continue
			}
			if !exists {
				_, err = d.conn.Create(path, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
				if err != nil && err != zk.ErrNodeExists {
					d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to recreate instance: %v", err))
				}
			}
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeInfo, "Stopping keepalive")
			return
		}
	}
}

// Deregister .
func (d *ZooKeeperDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	path := path.Join("/services", serviceName, instanceID)
	err := d.conn.Delete(path, -1)
	if err != nil && err != zk.ErrNoNode {
		return fmt.Errorf("failed to deregister instance: %v", err)
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *ZooKeeperDiscovery) Watch(ctx context.Context, serviceName string) (chan []types.Instance, error) {
	prefix := path.Join("/services", serviceName)
	ch := make(chan []types.Instance, 1)
	go func() {
		defer close(ch)
		for {
			instances, err := d.GetInstances(serviceName)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Failed to get instances: %v", err))
				time.Sleep(time.Second)
				continue
			}
			select {
			case ch <- instances:
			case <-ctx.Done():
				return
			}
			_, _, wch, err := d.conn.ChildrenW(prefix)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("Failed to watch children: %v", err))
				time.Sleep(time.Second)
				continue
			}
			select {
			case <-wch:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// GetInstances .
func (d *ZooKeeperDiscovery) GetInstances(serviceName string) ([]types.Instance, error) {
	prefix := path.Join("/services", serviceName)
	children, _, err := d.conn.Children(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	var instances []types.Instance
	for _, child := range children {
		data, _, err := d.conn.Get(path.Join(prefix, child))
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to get instance data: %v", err))
			continue
		}
		var inst types.Instance
		if err = json.Unmarshal(data, &inst); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to unmarshal instance: %v", err))
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// Close .
func (d *ZooKeeperDiscovery) Close() error {
	d.conn.Close()
	return nil
}
