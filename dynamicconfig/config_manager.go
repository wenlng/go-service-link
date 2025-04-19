/**
 * @Author Awen
 * @Date 2025/06/18
 * @Email wengaolng@gmail.com
 **/

package dynamicconfig

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/wenlng/go-service-discovery/dynamicconfig/provider"
	"github.com/wenlng/go-service-discovery/helper"
)

// ReloadCallback ..
type ReloadCallback func(key string, config *provider.Config) error

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

	outputLogCallback helper.OutputLogCallback
}

// NewConfigManager ..
func NewConfigManager(provider provider.ConfigProvider, configs map[string]*provider.Config, keys []string) *ConfigManager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &ConfigManager{
		provider:   provider,
		configs:    configs,
		keys:       keys,
		ctx:        ctx,
		cancel:     cancel,
		healthFreq: 10 * time.Second,
	}
	m.startHealthCheck()
	return m
}

// SetOutputLogCallback Set the log out hook function
func (m *ConfigManager) SetOutputLogCallback(outputLogCallback helper.OutputLogCallback) {
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

// SyncConfig synchronous configuration
func (m *ConfigManager) SyncConfig(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, key := range m.keys {
		remote, err := m.provider.GetConfig(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to get config for key %s: %v", key, err)
		}

		localConfig := m.configs[key]

		if remote == nil {
			if err = localConfig.Validate(); err != nil {
				return fmt.Errorf("local config validation failed for key %s: %v", key, err)
			}

			if localConfig.Version <= 0 {
				localConfig.Version = time.Now().UnixNano()
			}

			if err = m.provider.PutConfig(ctx, key, localConfig); err != nil {
				return fmt.Errorf("failed to put config for key %s: %v", key, err)
			}

			m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Uploaded local config, key: %v", key))
		} else {
			if err = remote.Validate(); err != nil {
				return fmt.Errorf("remote config validation failed for key %s: %v", key, err)
			}

			if remote.Version > localConfig.Version {
				m.configs[key] = remote
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

						if err := config.Validate(); err != nil {
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
						m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Watch failed, attempting reconnect, key: %v", key))
						if err := m.reconnect(); err != nil {
							m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Reconnect failed, attempting reconnect, key: %v", key))
							time.Sleep(time.Second * 5)
							continue
						}
						if err = m.SyncConfig(m.ctx); err != nil {
							m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Resync failed after reconnect, key: %v", key))
						}
					}
					time.Sleep(time.Second)
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

// reconnect ..
func (m *ConfigManager) reconnect() error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = time.Minute
	return backoff.Retry(func() error {
		if err := m.provider.Reconnect(m.ctx); err != nil {
			m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Reconnect attempt failed, err: %v", err))
			return err
		}
		m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Reconnected successfully"))
		return nil
	}, backoff.WithContext(b, m.ctx))
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
						m.outLog(helper.OutputLogTypeInfo, fmt.Sprintf("[ConfigManager] Using cached health status, mertice: %v", status.Metrics))
						continue
					}
				}

				// Carry out health checks
				ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
				status := m.provider.HealthCheck(ctx)
				cancel()
				m.setCachedHealthStatus(status)

				if status.Err != nil {
					m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Health check failed, mertice: %v, err: %v", status.Metrics, status.Err))
					m.healthFreq = 2 * time.Second // Increase the frequency when failure occurs
					if err := m.reconnect(); err != nil {
						m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Reconnect failed, err: %v", err))
						continue
					}
					if err := m.SyncConfig(m.ctx); err != nil {
						m.outLog(helper.OutputLogTypeError, fmt.Sprintf("[ConfigManager] Resync failed after reconnect, err: %v", err))
					}
				} else {
					m.healthFreq = 10 * time.Second // Return to the normal frequency when successful
					m.outLog(helper.OutputLogTypeDebug, fmt.Sprintf("[ConfigManager] Health check passed, metrics: %v", status.Metrics))
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
