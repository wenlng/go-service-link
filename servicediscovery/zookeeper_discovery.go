/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package servicediscovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"path"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/google/uuid"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/helper"
)

// ZooKeeperDiscovery .
type ZooKeeperDiscovery struct {
	conn              *zk.Conn
	instanceID        string
	logOutputHookFunc LogOutputHookFunc
	config            ZooKeeperDiscoveryConfig
}

// ZooKeeperDiscoveryConfig .
type ZooKeeperDiscoveryConfig struct {
	Timeout           time.Duration
	MaxBufferSize     int
	MaxConnBufferSize int

	address        []string
	ttl            time.Duration
	keepAlive      time.Duration
	maxRetries     int
	baseRetryDelay time.Duration
	tlsConfig      *base.TLSConfig
	username       string
	password       string
}

// NewZooKeeperDiscovery .
func NewZooKeeperDiscovery(config ZooKeeperDiscoveryConfig) (*ZooKeeperDiscovery, error) {
	if config.maxRetries <= 0 {
		config.maxRetries = 3
	}

	if !helper.IsDurationSet(config.baseRetryDelay) {
		config.baseRetryDelay = 500 * time.Millisecond
	}

	conn, err := createZooKeeperClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ZooKeeper: %v", err)
	}

	return &ZooKeeperDiscovery{
		conn:   conn,
		config: config,
	}, nil
}

// tlsDialer create a dialer that supports TLS
func tlsDialer(tlsConfig *tls.Config) zk.Dialer {
	return func(network, address string, timeout time.Duration) (net.Conn, error) {
		if tlsConfig == nil {
			return net.DialTimeout(network, address, timeout)
		}

		dialer := &net.Dialer{Timeout: timeout}
		return tls.DialWithDialer(dialer, network, address, tlsConfig)
	}
}

// createNacosClient try to create a ZooKeeper client that includes a retry mechanism
func createZooKeeperClient(config ZooKeeperDiscoveryConfig) (*zk.Conn, error) {
	var conn *zk.Conn
	var err error
	var events <-chan zk.Event

	var tlsConf *tls.Config
	var daler zk.Dialer
	if config.tlsConfig != nil {
		tlsConf, err = base.CreateTLSConfig(config.tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("the TLS configuration cannot be created: %v", err)
		}
		daler = tlsDialer(tlsConf)
	}

	for attempt := 1; attempt <= config.maxRetries; attempt++ {
		if daler != nil {
			conn, events, err = zk.Connect(config.address, time.Second)
		} else {
			conn, events, err = zk.Connect(config.address, time.Second)
		}

		if err != nil {
			d := 1 << uint(attempt-1)
			delay := time.Duration(float64(config.baseRetryDelay) * float64(d))
			jitter := time.Duration(rand.Intn(100)) * time.Millisecond
			time.Sleep(delay + jitter)
			continue
		}

		timeout := time.After(5 * time.Second)
		for conn.State() != zk.StateHasSession {
			select {
			case <-events:
				continue
			case <-timeout:
				conn.Close()
				return nil, fmt.Errorf("the connection to ZooKeeper has timed out")
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		// Add authentication information (digest authentication)
		if config.username != "" || config.password != "" {
			auth := fmt.Sprintf("digest:%s:%s", config.username, config.password)
			err = conn.AddAuth("digest", []byte(auth))
			if err != nil {
				return nil, err
			}
		}

		return conn, nil
	}

	return nil, fmt.Errorf("after %d attempts, it still couldn't connect to ZooKeeper: %v", config.maxRetries, err)
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

// withRetry perform the operation using the retry logic
func (d *ZooKeeperDiscovery) withRetry(ctx context.Context, operation func() error) error {
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
			newConn, clientErr := createZooKeeperClient(d.config)
			if clientErr == nil {
				d.conn.Close()
				d.conn = newConn
				d.outLog(ServiceDiscoveryLogTypeInfo, "Successfully reconnected to ZooKeeper")
			}
		}
	}
	return fmt.Errorf("the operation failed after %d attempts: %v", d.config.maxRetries, err)
}

// Register .
func (d *ZooKeeperDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	if instanceID == "" {
		instanceID = uuid.New().String()
	}
	d.instanceID = instanceID

	data, err := json.Marshal(base.ServiceInstance{
		InstanceID: instanceID,
		Host:       host,
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

	p := path.Join("/services", serviceName, instanceID)
	err = d.ensureParentNodes(p)
	if err != nil {
		return err
	}

	err = d.withRetry(ctx, func() error {
		_, createErr := d.conn.Create(p, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
		if createErr != nil && createErr != zk.ErrNodeExists {
			return fmt.Errorf("the Zookeeper instance cannot be registered: %v", createErr)
		}
		return nil
	})
	if err != nil {
		return err
	}

	go d.keepAliveLoop(ctx, p, data)

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))
	return nil
}

// ensureParentNodes recursively create the parent node
func (d *ZooKeeperDiscovery) ensureParentNodes(targetPath string) error {
	parentPath := path.Dir(targetPath)
	if parentPath == "/" || parentPath == "." {
		return nil
	}

	var exists bool
	err := d.withRetry(context.Background(), func() error {
		var checkErr error
		exists, _, checkErr = d.conn.Exists(parentPath)
		return checkErr
	})
	if err != nil {
		return fmt.Errorf("failed to check the parent node %s: %v", parentPath, err)
	}
	if exists {
		return nil
	}

	if err = d.ensureParentNodes(parentPath); err != nil {
		return err
	}

	err = d.withRetry(context.Background(), func() error {
		_, createErr := d.conn.Create(parentPath, []byte{}, 0, zk.WorldACL(zk.PermAll))
		if createErr != nil && createErr != zk.ErrNodeExists {
			return fmt.Errorf("the parent node of Zookeeper cannot be created %s: %v", parentPath, createErr)
		}
		return nil
	})
	return nil
}

// keepAliveLoop .
func (d *ZooKeeperDiscovery) keepAliveLoop(ctx context.Context, path string, data []byte) {
	ticker := time.NewTicker(d.config.keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var exists bool
			err := d.withRetry(ctx, func() error {
				var checkErr error
				exists, _, checkErr = d.conn.Exists(path)
				return checkErr
			})
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to check instance: %v", err))
				continue
			}
			if !exists {
				err = d.withRetry(ctx, func() error {
					_, createErr := d.conn.Create(path, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
					if createErr != nil && createErr != zk.ErrNodeExists {
						return fmt.Errorf("the Zoopeeker instance cannot be recreated: %v", createErr)
					}
					return nil
				})
			}
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeInfo, "Stopping keepalive")
			return
		}
	}
}

// Deregister .
func (d *ZooKeeperDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	p := path.Join("/services", serviceName, instanceID)
	err := d.withRetry(ctx, func() error {
		deleteErr := d.conn.Delete(p, -1)
		if deleteErr != nil && deleteErr != zk.ErrNoNode {
			return fmt.Errorf("failed to deregister instance: %v", deleteErr)
		}
		return nil
	})
	if err != nil {
		return err
	}

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

	return nil
}

// Watch .
func (d *ZooKeeperDiscovery) Watch(ctx context.Context, serviceName string) (chan []base.ServiceInstance, error) {
	prefix := path.Join("/services", serviceName)
	ch := make(chan []base.ServiceInstance, 1)
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
func (d *ZooKeeperDiscovery) GetInstances(serviceName string) ([]base.ServiceInstance, error) {
	prefix := path.Join("/services", serviceName)
	var children []string
	err := d.withRetry(context.Background(), func() error {
		var getErr error
		children, _, getErr = d.conn.Children(prefix)
		return getErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	var instances []base.ServiceInstance
	for _, child := range children {
		data, _, err := d.conn.Get(path.Join(prefix, child))
		if err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("Failed to get instance data: %v", err))
			continue
		}
		var inst base.ServiceInstance
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
