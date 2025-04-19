/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package clientpool

import (
	"fmt"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/wenlng/go-service-discovery/extraconfig"
)

// ZooKeeperPool Manage the ZooKeeper connection pool
type ZooKeeperPool struct {
	conns  chan *zk.Conn
	config *extraconfig.ZooKeeperExtraConfig
}

// NewZooKeeperPool ..
func NewZooKeeperPool(poolSize int, serverAddrs []string, config *extraconfig.ZooKeeperExtraConfig) (*ZooKeeperPool, error) {
	var daler zk.Dialer
	if config.GetTLSConfig() != nil {
		daler = config.CreateTlsDialer()
	}

	conns := make(chan *zk.Conn, poolSize)
	for i := 0; i < poolSize; i++ {
		var conn *zk.Conn
		var events <-chan zk.Event
		var err error

		if daler != nil {
			conn, events, err = zk.ConnectWithDialer(serverAddrs, time.Second*5, daler)
		} else {
			conn, events, err = zk.Connect(serverAddrs, time.Second*5)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create zookeeper client: %v", err)
		}

		timeout := time.After(5 * time.Second)
		for conn.State() != zk.StateHasSession {
			select {
			case <-events:
				continue
			case <-timeout:
				conn.Close()
				return nil, fmt.Errorf("the connection to ZooKeeper has timed out")
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		err = config.MergeTo(conn)
		if err != nil {
			return nil, err
		}

		conns <- conn
	}
	return &ZooKeeperPool{conns: conns, config: config}, nil
}

// Get ..
func (p *ZooKeeperPool) Get() *zk.Conn {
	return <-p.conns
}

// Put ..
func (p *ZooKeeperPool) Put(conn *zk.Conn) {
	select {
	case p.conns <- conn:
	default:
		conn.Close()
	}
}

// Close ..
func (p *ZooKeeperPool) Close() {
	for len(p.conns) > 0 {
		conn := <-p.conns
		conn.Close()
	}
}
