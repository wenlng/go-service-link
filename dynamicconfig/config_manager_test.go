package dynamicconfig

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/wenlng/go-service-discovery/dynamicconfig/provider"
)

func TestConfigManager(t *testing.T) {
	configs := map[string]*provider.Config{
		"/config/my-app/main": {
			Name:    "my-service-app-main",
			Version: 2256136267083311000,
			Content: `{"AppName": "my-app-main", "Port": 8080, DebugMode: true }`,
		},
		"/config/my-app/db": {
			Name:    "my-service-app-db",
			Version: 2245136267083311000,
			Content: `{"AppName": "my-app-main", "Port": 3306 }`,
		},
	}

	keys := make([]string, 0)
	for key, _ := range configs {
		keys = append(keys, key)
	}

	// "etcd"， "consul", "zookeeper", "nacos"
	providerCfg := provider.ProviderConfig{
		Type:      provider.ProviderTypeEtcd,
		Endpoints: []string{"localhost:2379"},
	}

	p, err := provider.NewProvider(providerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v \n", err)
		return
	}

	manager := NewConfigManager(p, configs, keys)

	manager.Subscribe(func(key string, config *provider.Config) error {
		log.Println("Hot reload triggered", "key", key, "content", config.Content)
		if key == "/config/my-app/db" {
			if len(config.Content) <= 0 {
				return errors.New("invalid port number")
			}
			fmt.Fprintf(os.Stderr, "Reinitializing database connection, content: %v \n", config.Content)
		}
		return nil
	})
	manager.Subscribe(func(key string, config *provider.Config) error {
		if key == "/config/my-app/main" {
			panic("Simulated panic in callback")
		}
		return nil
	})

	if err = manager.SyncConfig(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to sync config: %v\n", err)
		return
	}

	if err := manager.Watch(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start watch: %v \n", err)
		return
	}

	for _, key := range keys {
		config := manager.GetLocalConfig(key)
		fmt.Printf("Current config for %s: %+v\n", key, config)
	}

	if err := manager.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to close: %v \n", err)
	}
}
