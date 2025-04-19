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
	"sync"
	"time"

	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/clientpool"
	"github.com/wenlng/go-service-discovery/extraconfig"
	"github.com/wenlng/go-service-discovery/helper"
	"go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdProvider implement the Etcd configuration center
type EtcdProvider struct {
	pool      *clientpool.EtcdPool
	endpoints []string
	mu        sync.Mutex

	config            EtcdProviderConfig
	outputLogCallback helper.OutputLogCallback
}

// EtcdProviderConfig ..
type EtcdProviderConfig struct {
	address   []string
	poolSize  int
	tlsConfig *base.TLSConfig
	username  string
	password  string

	EtcdExtraConfig extraconfig.EtcdExtraConfig
}

// NewEtcdProvider ..
func NewEtcdProvider(conf EtcdProviderConfig) (ConfigProvider, error) {
	if conf.poolSize <= 0 {
		conf.poolSize = 5
	}

	if conf.username != "" {
		conf.EtcdExtraConfig.Username = conf.username
	}
	if conf.password != "" {
		conf.EtcdExtraConfig.Password = conf.password
	}
	if conf.tlsConfig != nil {
		conf.EtcdExtraConfig.SetTLSConfig(conf.tlsConfig)
	}

	pool, err := clientpool.NewEtcdPool(conf.poolSize, conf.address, &conf.EtcdExtraConfig)
	if err != nil {
		return nil, err
	}
	return &EtcdProvider{pool: pool, endpoints: conf.address, config: conf}, nil
}

// SetOutputLogCallback Set the log out hook function
func (p *EtcdProvider) SetOutputLogCallback(outputLogCallback helper.OutputLogCallback) {
	p.outputLogCallback = outputLogCallback
}

// outLog
func (p *EtcdProvider) outLog(logType helper.OutputLogType, message string) {
	if p.outputLogCallback != nil {
		p.outputLogCallback(logType, message)
	}
}

// GetConfig ..
func (p *EtcdProvider) GetConfig(ctx context.Context, key string) (*Config, error) {
	cli := p.pool.Get()
	defer p.pool.Put(cli)

	var config *Config
	operation := func() error {
		resp, err := cli.Get(ctx, key)
		if err != nil {
			return err
		}
		if len(resp.Kvs) == 0 {
			config = nil
			return nil
		}
		config = &Config{}
		return json.Unmarshal(resp.Kvs[0].Value, config)
	}

	if err := helper.WithRetry(ctx, operation); err != nil {
		return nil, fmt.Errorf("failed to get config: %v", err)
	}
	return config, nil
}

// PutConfig ..
func (p *EtcdProvider) PutConfig(ctx context.Context, key string, config *Config) error {
	cli := p.pool.Get()
	defer p.pool.Put(cli)

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	session, err := concurrency.NewSession(cli, concurrency.WithTTL(5))
	if err != nil {
		return fmt.Errorf("failed to create etcd session: %v", err)
	}
	defer session.Close()

	lockKey := key + "_lock"
	mutex := concurrency.NewMutex(session, lockKey)
	if err = mutex.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire etcd lock: %v", err)
	}
	defer mutex.Unlock(ctx)

	operation := func() error {
		_, err = cli.Put(ctx, key, string(data))
		return err
	}
	if err = helper.WithRetry(ctx, operation); err != nil {
		return fmt.Errorf("failed to put config: %v", err)
	}

	p.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[EtcdProvider] Config written with lock, key: %v", key))
	return nil
}

// WatchConfig ..
func (p *EtcdProvider) WatchConfig(ctx context.Context, key string, callback func(*Config)) error {
	cli := p.pool.Get()
	defer p.pool.Put(cli)
	watchChan := cli.Watch(ctx, key)

	go func() {
		for {
			select {
			case resp, ok := <-watchChan:
				if !ok {
					p.outLog(helper.OutputLogTypeWarn, fmt.Sprintf("[EtcdProvider] Etcd watch channel closed, key: %v", key))
					return
				}

				for _, ev := range resp.Events {
					if ev.Type == clientv3.EventTypePut {
						var config Config
						if err := json.Unmarshal(ev.Kv.Value, &config); err == nil {
							callback(&config)
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

// Reconnect ..
func (p *EtcdProvider) Reconnect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pool.Close()

	pool, err := clientpool.NewEtcdPool(p.config.poolSize, p.config.address, &p.config.EtcdExtraConfig)
	if err != nil {
		return fmt.Errorf("failed to reconnect etcd: %v", err)
	}
	p.pool = pool

	p.outLog(helper.OutputLogTypeWarn, "[EtcdProvider] Etcd reconnected successfully")
	return nil
}

// HealthCheck ..
func (p *EtcdProvider) HealthCheck(ctx context.Context) HealthStatus {
	cli := p.pool.Get()
	defer p.pool.Put(cli)
	metrics := make(map[string]interface{})
	start := time.Now()

	// Check the connection status
	resp, err := cli.Get(ctx, "/health", clientv3.WithLimit(1))
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd health check failed: %v", err)}
	}
	metrics["latency_ms"] = time.Since(start).Milliseconds()
	metrics["node_count"] = len(resp.Kvs)

	// Check the writing capability
	testKey := "/health/test"
	_, err = cli.Put(ctx, testKey, "test")
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd write check failed: %v", err)}
	}
	_, err = cli.Delete(ctx, testKey)
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd delete check failed: %v", err)}
	}

	// Check the monitoring interface
	respHTTP, err := http.Get(p.endpoints[0] + "/health")
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd health endpoint failed: %v", err)}
	}
	defer respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusOK {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd health endpoint returned %d", respHTTP.StatusCode)}
	}

	// Check the status of the cluster
	clusterResp, err := cli.Status(ctx, p.endpoints[0])
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd cluster status check failed: %v", err)}
	}
	metrics["leader"] = clusterResp.Leader

	// Obtain member information
	memberResp, err := cli.MemberList(ctx)
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("etcd member list check failed: %v", err)}
	}
	metrics["members"] = len(memberResp.Members)
	return HealthStatus{Metrics: metrics}
}

// Close ..
func (p *EtcdProvider) Close() error {
	p.pool.Close()
	return nil
}
