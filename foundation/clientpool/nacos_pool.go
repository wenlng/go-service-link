/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package clientpool

import (
	"fmt"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/wenlng/go-service-link/foundation/extraconfig"
	"github.com/wenlng/go-service-link/foundation/helper"
)

// NacosConfigPool manage the Nacos connection pool
type NacosConfigPool struct {
	clientChans chan config_client.IConfigClient
	config      *extraconfig.NacosExtraConfig
}

// NewNacosConfigPool ..
func NewNacosConfigPool(poolSize int, serverAddrs []string, config *extraconfig.NacosExtraConfig) (*NacosConfigPool, error) {
	clientChans := make(chan config_client.IConfigClient, poolSize)

	var sCfg = make([]constant.ServerConfig, len(serverAddrs))
	for index, addr := range serverAddrs {
		addrs := addr
		host, port, err := helper.SplitHostPort(addrs)
		if err != nil {
			return nil, fmt.Errorf("failed to create nacos client: %v", err)
		}

		sCfg[index] = constant.ServerConfig{
			IpAddr: host,
			Port:   uint64(port),
		}
	}

	cCfg := *constant.NewClientConfig(
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
	)

	err := config.MergeTo(&cCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to set nacos config: %v", err)
	}

	for i := 0; i < poolSize; i++ {
		client, err := clients.CreateConfigClient(map[string]interface{}{
			constant.KEY_SERVER_CONFIGS: sCfg,
			constant.KEY_CLIENT_CONFIG:  cCfg,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create nacos client: %v", err)
		}
		clientChans <- client
	}
	return &NacosConfigPool{clientChans: clientChans, config: config}, nil
}

// Get ..
func (p *NacosConfigPool) Get() config_client.IConfigClient {
	return <-p.clientChans
}

// Put ..
func (p *NacosConfigPool) Put(client config_client.IConfigClient) {
	select {
	case p.clientChans <- client:
	default:
		// @Pass
	}
}

// Close ..
func (p *NacosConfigPool) Close() {
	for len(p.clientChans) > 0 {
		<-p.clientChans
	}
}

/////////////////////////////////////////////////////////////////

// NacosNamingPool manage the Nacos connection pool
type NacosNamingPool struct {
	clientChans chan naming_client.INamingClient
	config      *extraconfig.NacosExtraConfig
}

// NewNacosNamingPool ..
func NewNacosNamingPool(poolSize int, serverAddrs []string, config *extraconfig.NacosExtraConfig) (*NacosNamingPool, error) {
	clientChans := make(chan naming_client.INamingClient, poolSize)

	var sCfg = make([]constant.ServerConfig, len(serverAddrs))
	for index, addr := range serverAddrs {
		addrs := addr
		host, port, err := helper.SplitHostPort(addrs)
		if err != nil {
			return nil, fmt.Errorf("failed to create nacos client: %v", err)
		}

		sCfg[index] = *constant.NewServerConfig(host, uint64(port))
	}

	cCfg := *constant.NewClientConfig(
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
	)

	err := config.MergeTo(&cCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to set nacos config: %v", err)
	}

	for i := 0; i < poolSize; i++ {
		client, err := clients.NewNamingClient(vo.NacosClientParam{
			ClientConfig:  &cCfg,
			ServerConfigs: sCfg,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create nacos client: %v", err)
		}
		clientChans <- client
	}
	return &NacosNamingPool{clientChans: clientChans, config: config}, nil
}

// Get ..
func (p *NacosNamingPool) Get() naming_client.INamingClient {
	return <-p.clientChans
}

// Put ..
func (p *NacosNamingPool) Put(client naming_client.INamingClient) {
	select {
	case p.clientChans <- client:
	default:
		// @Pass
	}
}

// Close ..
func (p *NacosNamingPool) Close() {
	for len(p.clientChans) > 0 {
		<-p.clientChans
	}
}
