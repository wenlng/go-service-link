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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
)

// ConsulProvider implement the Consul configuration center
type ConsulProvider struct {
	pool    *clientpool.ConsulPool
	address []string
	mu      sync.Mutex

	config            ConsulProviderConfig
	outputLogCallback helper.OutputLogCallback
}

// ConsulProviderConfig ..
type ConsulProviderConfig struct {
	address   []string
	poolSize  int
	tlsConfig *base.TLSConfig
	username  string
	password  string

	ConsulExtraConfig extraconfig.ConsulExtraConfig
}

// NewConsulProvider ..
func NewConsulProvider(conf ConsulProviderConfig) (ConfigProvider, error) {
	if conf.poolSize <= 0 {
		conf.poolSize = 5
	}

	if conf.username != "" {
		conf.ConsulExtraConfig.Username = conf.username
	}
	if conf.password != "" {
		conf.ConsulExtraConfig.Password = conf.password
	}
	if conf.tlsConfig != nil {
		conf.ConsulExtraConfig.SetTLSConfig(conf.tlsConfig)
	}

	pool, err := clientpool.NewConsulPool(conf.poolSize, conf.address, &conf.ConsulExtraConfig)
	if err != nil {
		return nil, err
	}
	return &ConsulProvider{pool: pool, address: conf.address, config: conf}, nil
}

// SetOutputLogCallback Set the log out hook function
func (p *ConsulProvider) SetOutputLogCallback(outputLogCallback helper.OutputLogCallback) {
	p.outputLogCallback = outputLogCallback
}

// outLog
func (p *ConsulProvider) outLog(logType helper.OutputLogType, message string) {
	if p.outputLogCallback != nil {
		p.outputLogCallback(logType, message)
	}
}

// GetConfig ..
func (p *ConsulProvider) GetConfig(ctx context.Context, key string) (*Config, error) {
	key = strings.TrimPrefix(key, "/")
	cli := p.pool.Get()
	defer p.pool.Put(cli)

	var config *Config
	operation := func() error {
		kv, _, err := cli.KV().Get(key, nil)
		if err != nil {
			return err
		}
		if kv == nil {
			config = nil
			return nil
		}
		config = &Config{}
		return json.Unmarshal(kv.Value, config)
	}

	if err := helper.WithRetry(ctx, operation); err != nil {
		return nil, fmt.Errorf("failed to get config: %v", err)
	}
	return config, nil
}

// PutConfig ..
func (p *ConsulProvider) PutConfig(ctx context.Context, key string, config *Config) error {
	key = strings.TrimPrefix(key, "/")
	cli := p.pool.Get()
	defer p.pool.Put(cli)

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	lockKey := strings.TrimPrefix(key, "/") + "_lock"
	lock, err := cli.LockKey(lockKey)
	if err != nil {
		return fmt.Errorf("failed to create consul lock: %v", err)
	}
	defer lock.Unlock()

	if _, err = lock.Lock(nil); err != nil {
		return fmt.Errorf("failed to acquire consul lock: %v", err)
	}

	operation := func() error {
		_, err = cli.KV().Put(&api.KVPair{Key: key, Value: data}, nil)
		return err
	}
	if err = helper.WithRetry(ctx, operation); err != nil {
		return fmt.Errorf("failed to put config: %v", err)
	}

	p.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConsulProvider] Config written with lock, key: %v", key))
	return nil
}

// WatchConfig ..
func (p *ConsulProvider) WatchConfig(ctx context.Context, key string, callback func(*Config)) error {
	key = strings.TrimPrefix(key, "/")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				cli := p.pool.Get()
				kv, _, err := cli.KV().Get(key, nil)
				if err != nil {
					p.outLog(helper.OutputLogTypeWarn, fmt.Sprintf("[ConsulProvider] Consul watch error, key: %v", key))
					p.pool.Put(cli)
					time.Sleep(time.Second)
					continue
				}
				if kv != nil {
					var config Config
					if err := json.Unmarshal(kv.Value, &config); err == nil {
						callback(&config)
					}
				}
				p.pool.Put(cli)
				time.Sleep(time.Second)
			}
		}
	}()
	return nil
}

// Reconnect ..
func (p *ConsulProvider) Reconnect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pool.Close()

	pool, err := clientpool.NewConsulPool(p.config.poolSize, p.config.address, &p.config.ConsulExtraConfig)
	if err != nil {
		return fmt.Errorf("failed to reconnect consul: %v", err)
	}
	p.pool = pool
	p.outLog(helper.OutputLogTypeInfo, "[ConsulProvider] Consul reconnected successfully")
	return nil
}

// HealthCheck ..
func (p *ConsulProvider) HealthCheck(ctx context.Context) HealthStatus {
	cli := p.pool.Get()
	defer p.pool.Put(cli)
	metrics := make(map[string]interface{})
	start := time.Now()

	// Check the status of the leader
	leader, err := cli.Status().Leader()
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("consul health check failed: %v", err)}
	}
	metrics["latency_ms"] = time.Since(start).Milliseconds()
	metrics["leader"] = leader

	// Check the writing capability
	testKey := "health/test"
	_, err = cli.KV().Put(&api.KVPair{Key: testKey, Value: []byte("test")}, nil)
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("consul write check failed: %v", err)}
	}
	_, err = cli.KV().Delete(testKey, nil)
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("consul delete check failed: %v", err)}
	}

	// Check the monitoring interface
	respHTTP, err := http.Get("http://" + p.address[0] + "/v1/health/service/consul")
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("consul health endpoint failed: %v", err)}
	}
	defer respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusOK {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("consul health endpoint returned %d", respHTTP.StatusCode)}
	}

	// Check the status of the cluster
	peers, err := cli.Status().Peers()
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("consul peers check failed: %v", err)}
	}
	metrics["peers"] = len(peers)
	return HealthStatus{Metrics: metrics}
}

// Close ..
func (p *ConsulProvider) Close() error {
	p.pool.Close()
	return nil
}
