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

	"github.com/go-zookeeper/zk"
	"github.com/google/uuid"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
)

// ZooKeeperDiscovery .
type ZooKeeperDiscovery struct {
	//conn              *zk.Conn
	instanceID        string
	pool              *clientpool.ZooKeeperPool
	outputLogCallback OutputLogCallback
	config            ZooKeeperDiscoveryConfig

	registeredServices map[string]registeredServiceInfo
	mutex              sync.RWMutex
}

// ZooKeeperDiscoveryConfig .
type ZooKeeperDiscoveryConfig struct {
	extraconfig.ZooKeeperExtraConfig

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

// NewZooKeeperDiscovery .
func NewZooKeeperDiscovery(config ZooKeeperDiscoveryConfig) (*ZooKeeperDiscovery, error) {
	if config.poolSize <= 0 {
		config.poolSize = 5
	}

	if config.maxRetries <= 0 {
		config.maxRetries = 3
	}

	if !helper.IsDurationSet(config.baseRetryDelay) {
		config.baseRetryDelay = 500 * time.Millisecond
	}

	pool, err := clientpool.NewZooKeeperPool(config.poolSize, config.address, &config.ZooKeeperExtraConfig)
	if err != nil {
		return nil, err
	}

	return &ZooKeeperDiscovery{
		config:             config,
		pool:               pool,
		registeredServices: make(map[string]registeredServiceInfo),
	}, nil
}

// SetOutputLogCallback .
func (d *ZooKeeperDiscovery) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
	d.outputLogCallback = outputLogCallback
}

// outLog
func (d *ZooKeeperDiscovery) outLog(logType ServiceDiscoveryLogType, message string) {
	if d.outputLogCallback != nil {
		d.outputLogCallback(logType, message)
	}
}

// checkAndReRegisterServices check and re-register the service
func (d *ZooKeeperDiscovery) checkAndReRegisterServices(ctx context.Context) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	d.mutex.RLock()
	services := make(map[string]registeredServiceInfo)
	for k, v := range d.registeredServices {
		services[k] = v
	}
	d.mutex.RUnlock()

	for instanceID, svcInfo := range services {
		p := path.Join("/services", svcInfo.ServiceName, instanceID)
		operation := func() error {
			exists, _, err := cli.Exists(p)
			if err != nil {
				return fmt.Errorf("failed to check the service registration status: %v", err)
			}
			if !exists {
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
func (d *ZooKeeperDiscovery) Register(ctx context.Context, serviceName, instanceID, host, httpPort, grpcPort string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

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

	operation := func() error {
		_, createErr := cli.Create(p, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
		if createErr != nil && createErr != zk.ErrNodeExists {
			return fmt.Errorf("the Zookeeper instance cannot be registered: %v", createErr)
		}
		return nil
	}
	if err = helper.WithRetry(context.Background(), operation); err != nil {
		return err
	}

	go d.keepAliveLoop(ctx, p, data)

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
		fmt.Sprintf("[ZooKeeperDiscovery] Registered instance, service: %s, instanceId: %s, host: %s, http_port: %s, grpc_port: %s",
			serviceName, instanceID, host, httpPort, grpcPort))

	return nil
}

// ensureParentNodes recursively create the parent node
func (d *ZooKeeperDiscovery) ensureParentNodes(targetPath string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	parentPath := path.Dir(targetPath)
	if parentPath == "/" || parentPath == "." {
		return nil
	}

	var exists bool
	operation := func() error {
		var checkErr error
		exists, _, checkErr = cli.Exists(parentPath)
		return checkErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		return fmt.Errorf("failed to check the parent node %s: %v", parentPath, err)
	}

	if exists {
		return nil
	}

	if err := d.ensureParentNodes(parentPath); err != nil {
		return err
	}

	operation = func() error {
		_, createErr := cli.Create(parentPath, []byte{}, 0, zk.WorldACL(zk.PermAll))
		if createErr != nil && createErr != zk.ErrNodeExists {
			return fmt.Errorf("the parent node of Zookeeper cannot be created %s: %v", parentPath, createErr)
		}
		return nil
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		return err
	}

	return nil
}

// keepAliveLoop .
func (d *ZooKeeperDiscovery) keepAliveLoop(ctx context.Context, path string, data []byte) {
	ticker := time.NewTicker(d.config.keepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cli := d.pool.Get()
			var exists bool
			operation := func() error {
				var checkErr error
				exists, _, checkErr = cli.Exists(path)
				return checkErr
			}
			if err := helper.WithRetry(context.Background(), operation); err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ZooKeeperDiscovery] Failed to check instance: %v", err))
				if err := d.checkAndReRegisterServices(ctx); err != nil {
					d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
				}
				d.pool.Put(cli)
				continue
			}

			if !exists {
				operation = func() error {
					_, createErr := cli.Create(path, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
					if createErr != nil && createErr != zk.ErrNodeExists {
						return fmt.Errorf("the Zoopeeker instance cannot be recreated: %v", createErr)
					}
					return nil
				}
				if err := helper.WithRetry(context.Background(), operation); err != nil {
					d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ZooKeeperDiscovery] Failed to check instance: %v", err))
				}
			}
			d.pool.Put(cli)
		case <-ctx.Done():
			d.outLog(ServiceDiscoveryLogTypeInfo, "Stopping keepalive")
			return
		}
	}
}

// Deregister .
func (d *ZooKeeperDiscovery) Deregister(ctx context.Context, serviceName, instanceID string) error {
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	p := path.Join("/services", serviceName, instanceID)
	operation := func() error {
		deleteErr := cli.Delete(p, -1)
		if deleteErr != nil && deleteErr != zk.ErrNoNode {
			return fmt.Errorf("failed to deregister instance: %v", deleteErr)
		}
		return nil
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		return err
	}

	d.mutex.Lock()
	delete(d.registeredServices, instanceID)
	d.mutex.Unlock()

	d.outLog(
		ServiceDiscoveryLogTypeInfo,
		fmt.Sprintf("[ZooKeeperDiscovery] Deregistered instance, service: %s, instanceId: %s", serviceName, instanceID))

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
				d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("[ZooKeeperDiscovery] Failed to get instances: %v", err))
				time.Sleep(time.Second)
				continue
			}
			select {
			case ch <- instances:
			case <-ctx.Done():
				return
			}
			cli := d.pool.Get()
			_, _, wch, err := cli.ChildrenW(prefix)
			d.pool.Put(cli)
			if err != nil {
				d.outLog(ServiceDiscoveryLogTypeError, fmt.Sprintf("[ZooKeeperDiscovery] Failed to watch children: %v", err))
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
	cli := d.pool.Get()
	defer d.pool.Put(cli)

	prefix := path.Join("/services", serviceName)

	var children []string
	operation := func() error {
		var getErr error
		children, _, getErr = cli.Children(prefix)
		return getErr
	}
	if err := helper.WithRetry(context.Background(), operation); err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}

	var instances []base.ServiceInstance
	for _, child := range children {
		data, _, err := cli.Get(path.Join(prefix, child))
		if err != nil {
			if err = d.checkAndReRegisterServices(context.Background()); err != nil {
				d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("The re-registration service failed: %v", err))
			}

			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ZooKeeperDiscovery] Failed to get instance data: %v", err))
			continue
		}
		var inst base.ServiceInstance
		if err = json.Unmarshal(data, &inst); err != nil {
			d.outLog(ServiceDiscoveryLogTypeWarn, fmt.Sprintf("[ZooKeeperDiscovery] Failed to unmarshal instance: %v", err))
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// Close .
func (d *ZooKeeperDiscovery) Close() error {
	d.pool.Close()
	return nil
}
