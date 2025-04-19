/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package extraconfig

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/wenlng/go-service-discovery/base"
)

// ZooKeeperExtraConfig .
type ZooKeeperExtraConfig struct {
	Username          string
	Password          string
	Timeout           time.Duration
	MaxBufferSize     int
	MaxConnBufferSize int
	tlsConfig         *base.TLSConfig
}

// SetTLSConfig .
func (ec *ZooKeeperExtraConfig) SetTLSConfig(tls *base.TLSConfig) {
	ec.tlsConfig = tls
}

// GetTLSConfig .
func (ec *ZooKeeperExtraConfig) GetTLSConfig() *base.TLSConfig {
	return ec.tlsConfig
}

// MergeTo .
func (ec *ZooKeeperExtraConfig) MergeTo(conn *zk.Conn) error {
	if ec.Username != "" || ec.Password != "" {
		auth := fmt.Sprintf("digest:%s:%s", ec.Username, ec.Password)
		err := conn.AddAuth("digest", []byte(auth))
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateTlsDialer .
func (ec *ZooKeeperExtraConfig) CreateTlsDialer() zk.Dialer {
	return func(network, address string, timeout time.Duration) (net.Conn, error) {
		if ec.tlsConfig == nil {
			return net.DialTimeout(network, address, timeout)
		}
		tlsConf, err := base.CreateTLSConfig(ec.tlsConfig)
		if err != nil {
			return nil, err
		}
		dialer := &net.Dialer{Timeout: timeout}
		return tls.DialWithDialer(dialer, network, address, tlsConf)
	}
}
