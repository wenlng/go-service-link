/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package extraconfig

import (
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/wenlng/go-service-link/foundation/common"
)

// NacosExtraConfig .
type NacosExtraConfig struct {
	Username             string
	Password             string
	TimeoutMs            uint64
	BeatInterval         int64
	NamespaceId          string
	AppName              string
	AppKey               string
	Endpoint             string
	RegionId             string
	AccessKey            string
	SecretKey            string
	OpenKMS              bool
	CacheDir             string
	DisableUseSnapShot   bool
	UpdateThreadNum      int
	NotLoadCacheAtStart  bool
	UpdateCacheWhenEmpty bool
	LogDir               string
	LogLevel             string
	ContextPath          string
	AppendToStdout       bool
	AsyncUpdateService   bool
	EndpointContextPath  string
	EndpointQueryParams  string
	ClusterName          string

	tlsConfig *common.TLSConfig
}

// SetTLSConfig ..
func (ec *NacosExtraConfig) SetTLSConfig(tls *common.TLSConfig) {
	ec.tlsConfig = tls
}

// MergeTo .
func (ec *NacosExtraConfig) MergeTo(destConfig *constant.ClientConfig) error {
	if ec.Username != "" {
		destConfig.Username = ec.Username
	}

	if ec.Password != "" {
		destConfig.Password = ec.Password
	}

	if ec.TimeoutMs > 0 {
		destConfig.TimeoutMs = ec.TimeoutMs
	}
	if ec.BeatInterval > 0 {
		destConfig.BeatInterval = ec.BeatInterval
	}
	if ec.NamespaceId != "" {
		destConfig.NamespaceId = ec.NamespaceId
	}
	if ec.AppName != "" {
		destConfig.AppName = ec.AppName
	}
	if ec.AppKey != "" {
		destConfig.AppKey = ec.AppKey
	}
	if ec.AppKey != "" {
		destConfig.AppKey = ec.AppKey
	}
	if ec.Endpoint != "" {
		destConfig.Endpoint = ec.Endpoint
	}
	if ec.RegionId != "" {
		destConfig.RegionId = ec.RegionId
	}
	if ec.AccessKey != "" {
		destConfig.AccessKey = ec.AccessKey
	}
	if ec.SecretKey != "" {
		destConfig.SecretKey = ec.SecretKey
	}
	if ec.OpenKMS {
		destConfig.OpenKMS = ec.OpenKMS
	}
	if ec.CacheDir != "" {
		destConfig.CacheDir = ec.CacheDir
	}
	if ec.DisableUseSnapShot {
		destConfig.DisableUseSnapShot = ec.DisableUseSnapShot
	}
	if ec.UpdateThreadNum > 0 {
		destConfig.UpdateThreadNum = ec.UpdateThreadNum
	}
	if ec.NotLoadCacheAtStart {
		destConfig.NotLoadCacheAtStart = ec.NotLoadCacheAtStart
	}
	if ec.UpdateCacheWhenEmpty {
		destConfig.UpdateCacheWhenEmpty = ec.UpdateCacheWhenEmpty
	}
	if ec.LogDir != "" {
		destConfig.LogDir = ec.LogDir
	}
	if ec.LogLevel != "" {
		destConfig.LogLevel = ec.LogLevel
	}
	if ec.ContextPath != "" {
		destConfig.ContextPath = ec.ContextPath
	}
	if ec.AppendToStdout {
		destConfig.AppendToStdout = ec.AppendToStdout
	}
	if ec.AsyncUpdateService {
		destConfig.AsyncUpdateService = ec.AsyncUpdateService
	}
	if ec.EndpointContextPath != "" {
		destConfig.EndpointContextPath = ec.EndpointContextPath
	}
	if ec.EndpointQueryParams != "" {
		destConfig.EndpointQueryParams = ec.EndpointQueryParams
	}
	if ec.ClusterName != "" {
		destConfig.ClusterName = ec.ClusterName
	}

	if ec.tlsConfig != nil {
		destConfig.TLSCfg = constant.TLSConfig{
			Enable:             true,
			CaFile:             ec.tlsConfig.CAFile,
			CertFile:           ec.tlsConfig.CertFile,
			KeyFile:            ec.tlsConfig.KeyFile,
			ServerNameOverride: ec.tlsConfig.ServerName,
		}
	}

	return nil
}
