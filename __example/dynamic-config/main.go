package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/wenlng/go-service-discovery/dynamicconfig"
	"github.com/wenlng/go-service-discovery/dynamicconfig/provider"
	"github.com/wenlng/go-service-discovery/helper"
)

func main() {
	configs := map[string]*provider.Config{
		"/config/my-app/main": {
			Name:    "my-service-app-main",
			Version: 0,
			//Version: 1856136267083311000,
			Content: `{"AppName": "my-app-main", "Port": 8081, DebugMode: false }`,
		},
		"/config/my-app/db": {
			Name:    "my-service-app-db",
			Version: 0,
			//Version: 1856136267083311000,
			Content: `{"AppName": "my-app-main", "Port": 3306 }`,
		},
	}

	keys := make([]string, 0)
	for key, _ := range configs {
		keys = append(keys, key)
	}

	providerCfg := provider.ProviderConfig{
		Type:      provider.ProviderTypeEtcd,
		Endpoints: []string{"localhost:2379"},

		//Type: provider.ProviderTypeConsul,
		//Endpoints: []string{
		//	"localhost:8500",
		//},

		//Type:      provider.ProviderTypeZookeeper,
		//Endpoints: []string{"localhost:2181"},

		//Type:      provider.ProviderTypeNacos,
		//Endpoints: []string{"localhost:8848"},
		//Username: "nacos",
		//Password: "nacos",
		//NacosProviderConfig: provider.NacosProviderConfig{
		//	NacosExtraConfig: extraconfig.NacosExtraConfig{
		//		NamespaceId: "",
		//	},
		//},
	}

	p, err := provider.NewProvider(providerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v \n", err)
		return
	}

	p.SetOutputLogCallback(func(logType helper.OutputLogType, message string) {
		if logType == helper.OutputLogTypeError {
			fmt.Fprintf(os.Stderr, message+"\n")
		} else {
			fmt.Fprintf(os.Stdout, message+"\n")
		}
	})

	manager := dynamicconfig.NewConfigManager(p, configs, keys)
	manager.SetOutputLogCallback(func(logType helper.OutputLogType, message string) {
		if logType == helper.OutputLogTypeError {
			fmt.Fprintf(os.Stderr, message+"\n")
		} else {
			fmt.Fprintf(os.Stdout, message+"\n")
		}
	})

	manager.Subscribe(func(key string, config *provider.Config) error {
		log.Println(">>>>>>>>>>>> Hot reload triggered", "key", key, "content", config.Content)
		if key == "/config/my-app/db" {
			if len(config.Content) <= 0 {
				return errors.New("invalid port number")
			}
			fmt.Fprintf(os.Stderr, ">>>>>>>>>>>>>>> Reinitializing database connection, content: %v \n", config.Content)
		}
		return nil
	})
	manager.Subscribe(func(key string, config *provider.Config) error {
		if key == "/config/my-app/main" {
			// test panic
			//panic("Simulated panic in callback")
		}
		return nil
	})

	if err = manager.SyncConfig(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to sync config: %v\n", err)
		return
	}

	go func() {
		for {
			time.Sleep(3 * time.Second)
			for _, key := range keys {
				config := manager.GetLocalConfig(key)
				fmt.Printf(">>> Current config for %s: %+v\n", key, config)
			}
		}
	}()

	if err := manager.Watch(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start watch: %v \n", err)
		return
	}

	fmt.Println(">>>>>>>> start watching ....")

	wg := sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()

	if err := manager.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to close: %v \n", err)
	}
}
