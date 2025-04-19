/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package provider

import (
	"context"
	"fmt"

	"github.com/wenlng/go-service-discovery/base"
	"github.com/wenlng/go-service-discovery/helper"
)

type ProviderType string

// ProviderType .
const (
	ProviderTypeEtcd      ProviderType = "etcd"
	ProviderTypeZookeeper              = "zookeeper"
	ProviderTypeConsul                 = "consul"
	ProviderTypeNacos                  = "nacos"
	ProviderTypeNone                   = "none"
)

// Config define the configuration structure
type Config struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
	Content string `json:"content"`
}

// Validate ...
func (c *Config) Validate() error {
	if c.Content == "" {
		return fmt.Errorf("content cannot be empty")
	}
	return nil
}

// HealthStatus define the results of health check-ups
type HealthStatus struct {
	Metrics map[string]interface{}
	Err     error
}

// ConfigProvider define the configuration center interface
type ConfigProvider interface {
	GetConfig(ctx context.Context, key string) (*Config, error)
	PutConfig(ctx context.Context, key string, config *Config) error
	WatchConfig(ctx context.Context, key string, callback func(*Config)) error
	Close() error
	Reconnect(ctx context.Context) error
	HealthCheck(ctx context.Context) HealthStatus
	SetOutputLogCallback(fn helper.OutputLogCallback)
}

// ProviderConfig ..
type ProviderConfig struct {
	Type      ProviderType // "etcd", "consul", "zookeeper", "nacos"
	Endpoints []string
	Username  string
	Password  string
	PoolSize  int
	tlsConfig *base.TLSConfig

	EtcdProviderConfig
	NacosProviderConfig
	ConsulProviderConfig
	ZooKeeperProviderConfig
}

// NewProvider dynamically creates configuration centers
func NewProvider(cfg ProviderConfig) (ConfigProvider, error) {
	switch cfg.Type {
	case ProviderTypeEtcd:
		config := cfg.EtcdProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.tlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize
		return NewEtcdProvider(config)
	case ProviderTypeConsul:
		config := cfg.ConsulProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.tlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize

		return NewConsulProvider(config)
	case ProviderTypeZookeeper:
		config := cfg.ZooKeeperProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.tlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize

		return NewZooKeeperProvider(config)
	case ProviderTypeNacos:
		config := cfg.NacosProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.tlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize

		return NewNacosProvider(config)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
}
