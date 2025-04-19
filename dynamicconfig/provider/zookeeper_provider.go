/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
)

// ZooKeeperProvider implement the ZookPeeper configuration center
type ZooKeeperProvider struct {
	pool    *clientpool.ZooKeeperPool
	address []string
	mu      sync.Mutex

	config            ZooKeeperProviderConfig
	outputLogCallback helper.OutputLogCallback
}

// ZooKeeperProviderConfig ..
type ZooKeeperProviderConfig struct {
	address   []string
	poolSize  int
	tlsConfig *base.TLSConfig
	username  string
	password  string

	ZooKeeperExtraConfig extraconfig.ZooKeeperExtraConfig
}

// NewZooKeeperProvider ..
func NewZooKeeperProvider(conf ZooKeeperProviderConfig) (ConfigProvider, error) {
	if conf.poolSize <= 0 {
		conf.poolSize = 3
	}

	if conf.username != "" {
		conf.ZooKeeperExtraConfig.Username = conf.username
	}
	if conf.password != "" {
		conf.ZooKeeperExtraConfig.Password = conf.password
	}
	if conf.tlsConfig != nil {
		conf.ZooKeeperExtraConfig.SetTLSConfig(conf.tlsConfig)
	}

	pool, err := clientpool.NewZooKeeperPool(conf.poolSize, conf.address, &conf.ZooKeeperExtraConfig)
	if err != nil {
		return nil, err
	}
	return &ZooKeeperProvider{pool: pool, address: conf.address, config: conf}, nil
}

// SetOutputLogCallback Set the log out hook function
func (p *ZooKeeperProvider) SetOutputLogCallback(outputLogCallback helper.OutputLogCallback) {
	p.outputLogCallback = outputLogCallback
}

// outLog
func (p *ZooKeeperProvider) outLog(logType helper.OutputLogType, message string) {
	if p.outputLogCallback != nil {
		p.outputLogCallback(logType, message)
	}
}

// ensurePath ..
func (p *ZooKeeperProvider) ensurePath(conn *zk.Conn, zkPath string) error {
	parts := strings.Split(strings.Trim(zkPath, "/"), "/")
	current := ""

	for i, part := range parts[:len(parts)-1] {
		current = current + "/" + part
		if i == 0 {
			current = "/" + part
		}
		exists, _, err := conn.Exists(current)
		if err != nil {
			return fmt.Errorf("failed to check path %s: %v", current, err)
		}
		if !exists {
			_, err = conn.Create(current, []byte{}, 0, zk.WorldACL(zk.PermAll))
			if err != nil && err != zk.ErrNodeExists {
				return fmt.Errorf("failed to create path %s: %v", current, err)
			}
		}
	}
	return nil
}

// GetConfig ..
func (p *ZooKeeperProvider) GetConfig(ctx context.Context, key string) (*Config, error) {
	conn := p.pool.Get()
	defer p.pool.Put(conn)
	var config *Config

	operation := func() error {
		data, _, err := conn.Get(key)
		if err != nil {
			if err == zk.ErrNoNode {
				config = nil
				return nil
			}
			return err
		}
		config = &Config{}
		return json.Unmarshal(data, config)
	}
	if err := helper.WithRetry(ctx, operation); err != nil {
		return nil, fmt.Errorf("failed to get config: %v", err)
	}

	return config, nil
}

// PutConfig ..
func (p *ZooKeeperProvider) PutConfig(ctx context.Context, key string, config *Config) error {
	conn := p.pool.Get()
	defer p.pool.Put(conn)
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	lockKey := key + "_lock"
	if err := p.ensurePath(conn, lockKey); err != nil {
		return fmt.Errorf("failed to ensure lock path %s: %v", lockKey, err)
	}

	lock := zk.NewLock(conn, lockKey, zk.WorldACL(zk.PermAll))
	if err = lock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire zookeeper lock: %v", err)
	}
	defer lock.Unlock()
	operation := func() error {
		exists, _, err := conn.Exists(key)
		if err != nil {
			return err
		}
		if !exists {
			_, err = conn.Create(key, data, 0, zk.WorldACL(zk.PermAll))
		} else {
			_, err = conn.Set(key, data, -1)
		}
		return err
	}
	if err = helper.WithRetry(ctx, operation); err != nil {
		return fmt.Errorf("failed to put config: %v", err)
	}

	p.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ZooKeeperProvider] Config written with lock, key: %v", key))
	return nil
}

// WatchConfig ..
func (p *ZooKeeperProvider) WatchConfig(ctx context.Context, key string, callback func(*Config)) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn := p.pool.Get()
				data, _, eventChan, err := conn.GetW(key)
				if err != nil {
					p.outLog(helper.OutputLogTypeWarn, fmt.Sprintf("[ZooKeeperProvider] ZooKeeper watch error, key: %v", key))
					p.pool.Put(conn)
					time.Sleep(time.Second)
					continue
				}
				if len(data) > 0 {
					var config Config
					if err = json.Unmarshal(data, &config); err == nil {
						callback(&config)
					}
				}
				p.pool.Put(conn)

				select {
				case ev := <-eventChan:
					if ev.Type == zk.EventNodeDataChanged {
						data, _, err := conn.Get(key)
						if err == nil {
							var config Config
							if err := json.Unmarshal(data, &config); err == nil {
								callback(&config)
							}
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return nil
}

// Reconnect ..
func (p *ZooKeeperProvider) Reconnect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pool.Close()

	pool, err := clientpool.NewZooKeeperPool(p.config.poolSize, p.address, &p.config.ZooKeeperExtraConfig)
	if err != nil {
		return fmt.Errorf("failed to reconnect zookeeper: %v", err)
	}
	p.pool = pool

	p.outLog(helper.OutputLogTypeInfo, "[ZooKeeperProvider] ZooKeeper reconnected successfully")
	return nil
}

// HealthCheck ..
func (p *ZooKeeperProvider) HealthCheck(ctx context.Context) HealthStatus {
	conn := p.pool.Get()
	defer p.pool.Put(conn)
	metrics := make(map[string]interface{})
	start := time.Now()

	// Check the connection status
	_, _, err := conn.Get("/zookeeper")
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("zookeeper health check failed: %v", err)}
	}
	metrics["latency_ms"] = time.Since(start).Milliseconds()

	// Check the writing capability
	testKey := "/health/test"
	_, err = conn.Create(testKey, []byte("test"), zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("zookeeper write check failed: %v", err)}
	}
	if err == nil {
		err = conn.Delete(testKey, -1)
		if err != nil {
			return HealthStatus{Metrics: metrics, Err: fmt.Errorf("zookeeper delete check failed: %v", err)}
		}
	}

	// Check the status of the cluster
	children, _, err := conn.Children("/zookeeper")
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("zookeeper cluster check failed: %v", err)}
	}
	metrics["members"] = len(children)
	return HealthStatus{Metrics: metrics}
}

// Close ..
func (p *ZooKeeperProvider) Close() error {
	p.pool.Close()
	return nil
}
