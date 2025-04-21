/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package extraconfig

import (
	"net/http"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/wenlng/go-service-link/foundation/common"
	"github.com/wenlng/go-service-link/foundation/helper"
)

// ConsulExtraConfig .
type ConsulExtraConfig struct {
	Username   string
	Password   string
	Scheme     string
	PathPrefix string
	Datacenter string
	Transport  *http.Transport
	HttpClient *http.Client
	WaitTime   time.Duration
	Token      string
	TokenFile  string
	Namespace  string
	Partition  string

	tlsConfig *common.TLSConfig
}

// SetTLSConfig .
func (ec *ConsulExtraConfig) SetTLSConfig(tls *common.TLSConfig) {
	ec.tlsConfig = tls
}

// MergeTo .
func (ec *ConsulExtraConfig) MergeTo(destConfig *api.Config) error {
	if ec.Username != "" || ec.Password != "" {
		destConfig.HttpAuth = &api.HttpBasicAuth{
			Username: ec.Username,
			Password: ec.Password,
		}
	}

	if ec.tlsConfig != nil {
		destConfig.TLSConfig = api.TLSConfig{
			Address:  ec.tlsConfig.Address,
			CAFile:   ec.tlsConfig.CAFile,
			KeyFile:  ec.tlsConfig.KeyFile,
			CertFile: ec.tlsConfig.CertFile,
		}
	}

	if helper.IsDurationSet(ec.WaitTime) {
		destConfig.WaitTime = ec.WaitTime
	}
	if ec.Scheme != "" {
		destConfig.Scheme = ec.Scheme
	}
	if ec.PathPrefix != "" {
		destConfig.PathPrefix = ec.PathPrefix
	}
	if ec.Datacenter != "" {
		destConfig.Datacenter = ec.Datacenter
	}
	if ec.Transport != nil {
		destConfig.Transport = ec.Transport
	}
	if ec.HttpClient != nil {
		destConfig.HttpClient = ec.HttpClient
	}
	if ec.Token != "" {
		destConfig.Token = ec.Token
	}
	if ec.TokenFile != "" {
		destConfig.TokenFile = ec.TokenFile
	}
	if ec.Namespace != "" {
		destConfig.Namespace = ec.Namespace
	}
	if ec.Partition != "" {
		destConfig.Partition = ec.Partition
	}

	return nil
}
