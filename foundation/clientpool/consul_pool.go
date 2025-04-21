/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package clientpool

import (
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/wenlng/go-service-link/foundation/extraconfig"
)

// ConsulPool manage the Consul connection pool
type ConsulPool struct {
	clients chan *api.Client
	config  *extraconfig.ConsulExtraConfig
}

// NewConsulPool ..
func NewConsulPool(poolSize int, serverAddrs []string, config *extraconfig.ConsulExtraConfig) (*ConsulPool, error) {
	pSize := poolSize
	if poolSize <= len(serverAddrs) {
		pSize = len(serverAddrs)
	}

	clients := make(chan *api.Client, pSize)

	for j := 0; j < pSize; j++ {
		cfg := api.DefaultConfig()
		err := config.MergeTo(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to set consul config: %v", err)
		}

		if j < len(serverAddrs) {
			cfg.Address = serverAddrs[j]
		} else if len(serverAddrs) > 0 {
			cfg.Address = serverAddrs[0]
		}

		cli, err := api.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create consul client: %v", err)
		}
		clients <- cli
	}

	return &ConsulPool{clients: clients}, nil
}

// Get ..
func (p *ConsulPool) Get() *api.Client {
	return <-p.clients
}

// Put ..
func (p *ConsulPool) Put(cli *api.Client) {
	select {
	case p.clients <- cli:
	default:
		// @Pass
	}
}

// Close ..
func (p *ConsulPool) Close() {
	for len(p.clients) > 0 {
		<-p.clients
	}
}
