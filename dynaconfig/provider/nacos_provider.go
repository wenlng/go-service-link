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

	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/wenlng/go-service-link/foundation/clientpool"
	"github.com/wenlng/go-service-link/foundation/common"
	"github.com/wenlng/go-service-link/foundation/extraconfig"
	"github.com/wenlng/go-service-link/foundation/helper"
)

// NacosProvider implement the Nacos configuration center
type NacosProvider struct {
	pool        *clientpool.NacosConfigPool
	serverAddrs []string
	mu          sync.Mutex

	config            NacosProviderConfig
	outputLogCallback helper.OutputLogCallback
}

// NacosProviderConfig ..
type NacosProviderConfig struct {
	address   []string
	poolSize  int
	tlsConfig *common.TLSConfig
	username  string
	password  string

	NacosExtraConfig extraconfig.NacosExtraConfig
}

// NewNacosProvider ..
func NewNacosProvider(conf NacosProviderConfig) (ConfigProvider, error) {
	if conf.NacosExtraConfig.NamespaceId == "" {
		conf.NacosExtraConfig.NamespaceId = "public"
	}
	if conf.poolSize <= 0 {
		conf.poolSize = 5
	}
	if conf.username != "" {
		conf.NacosExtraConfig.Username = conf.username
	}
	if conf.password != "" {
		conf.NacosExtraConfig.Password = conf.password
	}
	if conf.tlsConfig != nil {
		conf.NacosExtraConfig.SetTLSConfig(conf.tlsConfig)
	}

	pool, err := clientpool.NewNacosConfigPool(conf.poolSize, conf.address, &conf.NacosExtraConfig)
	if err != nil {
		return nil, err
	}
	return &NacosProvider{pool: pool, serverAddrs: conf.address, config: conf}, nil
}

// SetOutputLogCallback Set the log out hook function
func (p *NacosProvider) SetOutputLogCallback(outputLogCallback helper.OutputLogCallback) {
	p.outputLogCallback = outputLogCallback
}

// outLog
func (p *NacosProvider) outLog(logType helper.OutputLogType, message string) {
	if p.outputLogCallback != nil {
		p.outputLogCallback(logType, message)
	}
}

// GetConfig ..
func (p *NacosProvider) GetConfig(ctx context.Context, key string) (*Config, error) {
	key = strings.TrimPrefix(key, "/")
	key = strings.ReplaceAll(key, "/", ".")

	client := p.pool.Get()
	defer p.pool.Put(client)
	var config *Config
	operation := func() error {
		content, err := client.GetConfig(vo.ConfigParam{DataId: key, Group: "DEFAULT_GROUP"})
		if err != nil && !strings.Contains(err.Error(), "config data not exist") {
			return err
		}

		if content == "" {
			config = nil
			return nil
		}

		config = &Config{}
		return json.Unmarshal([]byte(content), config)
	}
	if err := helper.WithRetry(ctx, operation); err != nil {
		return nil, fmt.Errorf("failed to get config: %v", err)
	}
	return config, nil
}

// PutConfig ..
func (p *NacosProvider) PutConfig(ctx context.Context, key string, config *Config) error {
	key = strings.TrimPrefix(key, "/")
	key = strings.ReplaceAll(key, "/", ".")

	client := p.pool.Get()
	defer p.pool.Put(client)

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	operation := func() error {
		_, err = client.PublishConfig(vo.ConfigParam{
			DataId:  key,
			Group:   "DEFAULT_GROUP",
			Content: string(data),
		})
		return err
	}
	if err = helper.WithRetry(ctx, operation); err != nil {
		return fmt.Errorf("failed to put config: %v", err)
	}

	p.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[NacosProvider] Config written, key: %v", key))
	return nil
}

// WatchConfig ..
func (p *NacosProvider) WatchConfig(ctx context.Context, key string, callback func(*Config)) error {
	key = strings.TrimPrefix(key, "/")
	key = strings.ReplaceAll(key, "/", ".")

	client := p.pool.Get()
	defer p.pool.Put(client)

	err := client.ListenConfig(vo.ConfigParam{
		DataId: key,
		Group:  "DEFAULT_GROUP",
		OnChange: func(namespace, group, dataId, data string) {
			var config Config
			if err := json.Unmarshal([]byte(data), &config); err == nil {
				callback(&config)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to watch config: %v", err)
	}

	return nil
}

// HealthCheck ..
func (p *NacosProvider) HealthCheck(ctx context.Context) HealthStatus {
	client := p.pool.Get()
	defer p.pool.Put(client)
	metrics := make(map[string]interface{})
	start := time.Now()

	// Check the reading capability
	_, err := client.GetConfig(vo.ConfigParam{DataId: "health", Group: "DEFAULT_GROUP"})
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("nacos health check failed: %v", err)}
	}
	metrics["latency_ms"] = time.Since(start).Milliseconds()

	// Check the writing capability
	_, err = client.PublishConfig(vo.ConfigParam{DataId: "health.test", Group: "DEFAULT_GROUP", Content: "test"})
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("nacos write check failed: %v", err)}
	}
	_, err = client.DeleteConfig(vo.ConfigParam{DataId: "health.test", Group: "DEFAULT_GROUP"})
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("nacos delete check failed: %v", err)}
	}

	// Check the monitoring interface
	hUrl := helper.EnsureHTTP(p.serverAddrs[0] + "/nacos/actuator/health")
	respHTTP, err := http.Get(hUrl)
	if err != nil {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("nacos health endpoint failed: %v", err)}
	}
	defer respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusOK && respHTTP.StatusCode != http.StatusNotFound {
		return HealthStatus{Metrics: metrics, Err: fmt.Errorf("nacos health endpoint returned %d", respHTTP.StatusCode)}
	}
	return HealthStatus{Metrics: metrics}
}

// Close ..
func (p *NacosProvider) Close() error {
	p.pool.Close()
	return nil
}
