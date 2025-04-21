/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package dynaconfig

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wenlng/go-service-link/dynaconfig/provider"
	"github.com/wenlng/go-service-link/foundation/helper"
)

type OutputLogType = helper.OutputLogType

const (
	OutputLogTypeWarn  = helper.OutputLogTypeWarn
	OutputLogTypeInfo  = helper.OutputLogTypeInfo
	OutputLogTypeError = helper.OutputLogTypeError
	OutputLogTypeDebug = helper.OutputLogTypeDebug
)

// ReloadCallback ..
type ReloadCallback func(key string, config *provider.Config) error

type OutputLogCallback = helper.OutputLogCallback

// ConfigManager manage multi-configuration synchronization, listening and hot loading
type ConfigManager struct {
	provider     provider.ConfigProvider
	configs      map[string]*provider.Config
	keys         []string
	callbacks    []ReloadCallback
	mu           sync.RWMutex
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	healthFreq   time.Duration         // Health checkup frequency
	healthCache  provider.HealthStatus // Health check cache
	healthCacheT time.Time             // Cache time
	cacheMu      sync.RWMutex          // Cache lock
	cbsMu        sync.RWMutex          // Callback lock

	outputLogCallback OutputLogCallback
}

type ConfigManagerParams struct {
	ProviderConfig provider.ProviderConfig
	Configs        map[string]*provider.Config
}

// NewConfigManager ..
func NewConfigManager(cmp ConfigManagerParams) (*ConfigManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := provider.NewProvider(cmp.ProviderConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create provider, err: %v", err)
	}

	keys := make([]string, 0)
	for key, _ := range cmp.Configs {
		keys = append(keys, key)
	}

	m := &ConfigManager{
		provider:   p,
		configs:    cmp.Configs,
		keys:       keys,
		ctx:        ctx,
		cancel:     cancel,
		healthFreq: 10 * time.Second,
	}
	m.startHealthCheck()
	return m, nil
}

// SetOutputLogCallback Set the log out hook function
func (m *ConfigManager) SetOutputLogCallback(outputLogCallback OutputLogCallback) {
	m.provider.SetOutputLogCallback(outputLogCallback)
	m.outputLogCallback = outputLogCallback
}

// outLog
func (m *ConfigManager) outLog(logType helper.OutputLogType, message string) {
	if m.outputLogCallback != nil {
		m.outputLogCallback(logType, message)
	}
}

// Subscribe to the hot loading callback for configuration changes
func (m *ConfigManager) Subscribe(callback ReloadCallback) {
	m.cbsMu.Lock()
	defer m.cbsMu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// ASyncConfig synchronous configuration
func (m *ConfigManager) ASyncConfig(ctx context.Context) {
	go func() {
		err := m.SyncConfig(ctx)
		if err != nil {
			if m.outputLogCallback != nil {
				m.outputLogCallback(helper.OutputLogTypeError, fmt.Sprintf("async config err: %v", err))
			}
		}
	}()
}

// SyncConfig synchronous configuration
func (m *ConfigManager) SyncConfig(ctx context.Context) error {
	for _, key := range m.keys {
		remote, err := m.provider.GetConfig(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to get config for key %s: %v", key, err)
		}

		m.mu.RLock()
		localConfig := m.configs[key]
		m.mu.RUnlock()

		var skip bool

		if remote == nil {
			if skip, err = localConfig.CallValidate(); err != nil {
				if skip {
					continue
				}

				return fmt.Errorf("local config validation failed for key %s: %v", key, err)
			}

			if localConfig.Version <= 0 {
				//localConfig.Version = time.Now().UnixNano()
				localConfig.Version = 1
			}

			if err = m.provider.PutConfig(ctx, key, localConfig); err != nil {
				return fmt.Errorf("failed to put config for key %s: %v", key, err)
			}

			m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Uploaded local config, key: %v", key))
		} else {
			if skip, err = remote.CallValidate(); err != nil {
				if skip {
					continue
				}
				return fmt.Errorf("remote config validation failed for key %s: %v", key, err)
			}

			if remote.Version > localConfig.Version {
				m.mu.Lock()
				m.configs[key] = remote
				m.mu.Unlock()

				m.notifyCallbacks(key, remote)

				m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Synced remote config, key: %v", key))
			} else if remote.Version < localConfig.Version {
				if err = m.provider.PutConfig(ctx, key, localConfig); err != nil {
					return fmt.Errorf("failed to put config for key %s: %v", key, err)
				}

				m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Uploaded local config, key: %v", key))
			}
		}
	}

	return nil
}

// RefreshConfig sync refresh configuration
func (m *ConfigManager) RefreshConfig(ctx context.Context, key string, p *provider.Config) {
	if _, ok := m.configs[key]; !ok {
		return
	}

	m.mu.Lock()
	m.configs[key] = p
	m.mu.Unlock()

	m.ASyncConfig(ctx)
}

// Watch for configuration changes
func (m *ConfigManager) Watch() error {
	for _, k := range m.keys {
		key := k
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for {
				select {
				case <-m.ctx.Done():
					return
				default:
					err := m.provider.WatchConfig(m.ctx, key, func(config *provider.Config) {
						m.mu.Lock()
						defer m.mu.Unlock()

						if skip, err := config.CallValidate(); err != nil {
							if skip {
								return
							}

							m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Watched config validation failed, key: %v, err: %v", key, err))
							return
						}
						if config.Version > m.configs[key].Version {
							m.configs[key] = config
							m.notifyCallbacks(key, config)

							m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Updated local config, key: %v, config: %v", key, config))
						}
					})

					if err != nil {
						status, ok := m.getCachedHealthStatus()
						if !ok || status.Err != nil {
							continue
						}

						if err = m.SyncConfig(m.ctx); err != nil {
							m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Resync failed after reconnect, key: %v", key))
						}

						return
					} else {
						return
					}
				}
			}
		}()
	}
	return nil
}

// notifyCallbacks notify all subscribers to handle the callback error
func (m *ConfigManager) notifyCallbacks(key string, config *provider.Config) {
	m.cbsMu.RLock()
	defer m.cbsMu.RUnlock()
	for i, callback := range m.callbacks {
		go func(i int, cb ReloadCallback) {
			defer func() {
				if r := recover(); r != nil {
					m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Callback panicked, key: %v, err: %v", key, r))
				}
			}()
			if err := cb(key, config); err != nil {
				m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Callback failed, key: %v, err: %v", key, err))
			}
		}(i, callback)
	}
}

// getCachedHealthStatus obtain the cached health check results
func (m *ConfigManager) getCachedHealthStatus() (provider.HealthStatus, bool) {
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()

	if time.Since(m.healthCacheT) < 5*time.Second {
		return m.healthCache, true
	}

	return provider.HealthStatus{}, false
}

// setCachedHealthStatus set up the health check cache
func (m *ConfigManager) setCachedHealthStatus(status provider.HealthStatus) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	m.healthCache = status
	m.healthCacheT = time.Now()
}

// startHealthCheck start the health check
func (m *ConfigManager) startHealthCheck() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.healthFreq)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				if status, ok := m.getCachedHealthStatus(); ok {
					if status.Err == nil {
						m.outLog(helper.OutputLogTypeDebug, fmt.Sprintf("[ConfigManager] Using cached health status, mertice: %v", status.Metrics))
						continue
					}
				}

				// Carry out health checks
				ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
				status := m.provider.HealthCheck(ctx)
				cancel()
				m.setCachedHealthStatus(status)

				if status.Err != nil {
					m.outLog(helper.OutputLogTypeWarn, fmt.Sprintf("[ConfigManager] Health check failed, mertice: %v, err: %v", status.Metrics, status.Err))
					m.healthFreq = 2 * time.Second // Increase the frequency when failure occurs
					if err := m.SyncConfig(m.ctx); err != nil {
						m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Resync failed after reconnect, err: %v", err))
					}
				} else {
					m.healthFreq = 10 * time.Second // Return to the normal frequency when successful
					m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Health check passed, metrics: %v", status.Metrics))
				}
				ticker.Reset(m.healthFreq)
			}
		}
	}()
}

// GetLocalConfig obtain the local configuration
func (m *ConfigManager) GetLocalConfig(key string) *provider.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configs[key]
}

// Close ..
func (m *ConfigManager) Close() error {
	m.cancel()
	m.wg.Wait()

	if err := m.provider.Close(); err != nil {
		m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Failed to close provider, err: %v", err))
		return err
	}

	m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Config manager closed"))

	return nil
}
