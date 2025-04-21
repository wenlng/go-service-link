/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package provider

import (
	"context"
	"fmt"

	"github.com/wenlng/go-service-link/foundation/common"
	"github.com/wenlng/go-service-link/foundation/helper"
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
	Name             string                                      `json:"name"`
	Version          int64                                       `json:"version"`
	Content          interface{}                                 `json:"content"`
	ValidateCallback func(config *Config) (skip bool, err error) `json:"-"`
}

// CallValidate ...
func (c *Config) CallValidate() (skip bool, err error) {
	if c.ValidateCallback != nil {
		return c.ValidateCallback(c)
	}

	if helper.IsOnlyEmpty(c.Content) {
		return false, fmt.Errorf("content cannot be empty")
	}
	return false, nil
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
	TlsConfig *common.TLSConfig

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
		config.tlsConfig = cfg.TlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize
		return NewEtcdProvider(config)
	case ProviderTypeConsul:
		config := cfg.ConsulProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.TlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize

		return NewConsulProvider(config)
	case ProviderTypeZookeeper:
		config := cfg.ZooKeeperProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.TlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize

		return NewZooKeeperProvider(config)
	case ProviderTypeNacos:
		config := cfg.NacosProviderConfig

		config.address = cfg.Endpoints
		config.tlsConfig = cfg.TlsConfig
		config.username = cfg.Username
		config.password = cfg.Password
		config.poolSize = cfg.PoolSize

		return NewNacosProvider(config)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
}
