/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package extraconfig

import (
	"context"
	"time"

	"github.com/wenlng/go-service-link/foundation/common"
	"github.com/wenlng/go-service-link/foundation/helper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// EtcdExtraConfig .
type EtcdExtraConfig struct {
	Username              string
	Password              string
	AutoSyncInterval      time.Duration
	DialTimeout           time.Duration
	DialKeepAliveTime     time.Duration
	DialKeepAliveTimeout  time.Duration
	MaxCallSendMsgSize    int
	MaxCallRecvMsgSize    int
	RejectOldCluster      bool
	DialOptions           []grpc.DialOption
	Context               context.Context
	Logger                *zap.Logger
	LogConfig             *zap.Config
	PermitWithoutStream   bool
	MaxUnaryRetries       uint
	BackoffWaitBetween    time.Duration
	BackoffJitterFraction float64
	tlsConfig             *common.TLSConfig
}

// SetTLSConfig .
func (ec *EtcdExtraConfig) SetTLSConfig(tls *common.TLSConfig) {
	ec.tlsConfig = tls
}

// MergeTo .
func (ec *EtcdExtraConfig) MergeTo(destConfig *clientv3.Config) error {
	if ec.Username != "" {
		destConfig.Username = ec.Username
	}

	if ec.Password != "" {
		destConfig.Password = ec.Password
	}

	if helper.IsDurationSet(ec.AutoSyncInterval) {
		destConfig.AutoSyncInterval = ec.AutoSyncInterval
	}
	if helper.IsDurationSet(ec.DialTimeout) {
		destConfig.DialTimeout = ec.DialTimeout
	}
	if helper.IsDurationSet(ec.DialKeepAliveTime) {
		destConfig.DialKeepAliveTime = ec.DialKeepAliveTime
	}
	if helper.IsDurationSet(ec.DialKeepAliveTimeout) {
		destConfig.DialKeepAliveTimeout = ec.DialKeepAliveTimeout
	}
	if ec.MaxCallSendMsgSize > 0 {
		destConfig.MaxCallSendMsgSize = ec.MaxCallSendMsgSize
	}
	if ec.MaxCallRecvMsgSize > 0 {
		destConfig.MaxCallRecvMsgSize = ec.MaxCallRecvMsgSize
	}

	if ec.RejectOldCluster {
		destConfig.RejectOldCluster = ec.RejectOldCluster
	}
	if len(ec.DialOptions) > 0 {
		destConfig.DialOptions = ec.DialOptions
	}
	if ec.Context != nil {
		destConfig.Context = ec.Context
	}
	if ec.Logger != nil {
		destConfig.Logger = ec.Logger
	}
	if ec.LogConfig != nil {
		destConfig.LogConfig = ec.LogConfig
	}
	if ec.PermitWithoutStream {
		destConfig.PermitWithoutStream = ec.PermitWithoutStream
	}
	if ec.MaxUnaryRetries > 0 {
		destConfig.MaxUnaryRetries = ec.MaxUnaryRetries
	}
	if helper.IsDurationSet(ec.BackoffWaitBetween) {
		destConfig.BackoffWaitBetween = ec.BackoffWaitBetween
	}
	if ec.BackoffJitterFraction > 0.0 {
		destConfig.BackoffJitterFraction = ec.BackoffJitterFraction
	}

	if ec.tlsConfig != nil {
		var err error
		destConfig.TLS, err = common.CreateTLSConfig(ec.tlsConfig)
		if err != nil {
			return err
		}
	}

	return nil
}
