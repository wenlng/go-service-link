/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package clientpool

import (
	"fmt"
	"log"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/wenlng/go-service-link/foundation/extraconfig"
)

// ZooKeeperPool Manage the ZooKeeper connection pool
type ZooKeeperPool struct {
	conns  chan *zk.Conn
	config *extraconfig.ZooKeeperExtraConfig
}

// NewZooKeeperPool ..
func NewZooKeeperPool(poolSize int, serverAddrs []string, config *extraconfig.ZooKeeperExtraConfig) (*ZooKeeperPool, error) {
	conns := make(chan *zk.Conn, poolSize)
	zl := &extraconfig.Zlogger{OutLogCallback: func(format string, s ...interface{}) {
		olc := config.GetZlogger()
		if olc != nil {
			olc.Printf("[go-zookeeper/zk logger] "+format, s...)
		} else {
			log.Printf("[go-zookeeper/zk logger] "+format, s...)
		}
	}}

	for i := 0; i < poolSize; i++ {
		var daler zk.Dialer
		if config.GetTLSConfig() != nil {
			daler = config.CreateTlsDialer()
		}

		var conn *zk.Conn
		//var events <-chan zk.Event
		var err error

		if daler != nil {
			conn, _, err = zk.ConnectWithDialer(serverAddrs, time.Second*5, daler)
		} else {
			conn, _, err = zk.Connect(serverAddrs, time.Second*5)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create zookeeper client: %v", err)
		}

		conn.SetLogger(zl)

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
