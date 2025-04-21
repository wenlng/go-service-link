/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package clientpool

import (
	"fmt"
	"time"

	"github.com/wenlng/go-service-link/foundation/extraconfig"
	"go.etcd.io/etcd/client/v3"
)

// EtcdPool manage the Etcd connection pool
type EtcdPool struct {
	clients chan *clientv3.Client
	config  *extraconfig.EtcdExtraConfig
}

// NewEtcdPool ..
func NewEtcdPool(poolSize int, serverAddrs []string, config *extraconfig.EtcdExtraConfig) (*EtcdPool, error) {
	clients := make(chan *clientv3.Client, poolSize)
	for i := 0; i < poolSize; i++ {
		cfg := clientv3.Config{Endpoints: serverAddrs, DialTimeout: 5 * time.Second}
		err := config.MergeTo(&cfg)
		if err != nil {
			return nil, err
		}

		cli, err := clientv3.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create etcd client: %v", err)
		}
		clients <- cli
	}
	return &EtcdPool{clients: clients, config: config}, nil
}

// Get ..
func (p *EtcdPool) Get() *clientv3.Client {
	return <-p.clients
}

// Put ..
func (p *EtcdPool) Put(cli *clientv3.Client) {
	select {
	case p.clients <- cli:
	default:
		cli.Close()
	}
}

// Close ..
func (p *EtcdPool) Close() {
	for len(p.clients) > 0 {
		cli := <-p.clients
		cli.Close()
	}
}
