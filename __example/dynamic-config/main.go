package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wenlng/go-service-link/dynaconfig"
	"github.com/wenlng/go-service-link/dynaconfig/provider"
)

func main() {
	configs := map[string]*provider.Config{
		"/config/my-app/main": {
			Name: "my-service-app-main",
			//Version: 0,
			Version: 2676136267083311000,
			Content: `{"AppName": "my-app-main", "Port": 8081, DebugMode: false }`,
			ValidateCallback: func(config *provider.Config) (skip bool, err error) {
				if config.Content == "" {
					return false, fmt.Errorf("contnet must be not empty")
				}
				return true, nil
			},
		},
		"/config/my-app/db": {
			Name: "my-service-app-db",
			//Version: 0,
			Version: 2676136267083311000,
			Content: `{"AppName": "my-app-db", "Port": 3306 }`,
			ValidateCallback: func(config *provider.Config) (skip bool, err error) {
				if config.Content == "" {
					return false, fmt.Errorf("contnet must be not empty")
				}
				return true, nil
			},
		},
	}

	keys := make([]string, 0)
	for key, _ := range configs {
		keys = append(keys, key)
	}

	providerCfg := provider.ProviderConfig{
		//Type:      provider.ProviderTypeEtcd,
		//Endpoints: []string{"localhost:2379"},

		//Type:      provider.ProviderTypeConsul,
		//Endpoints: []string{"localhost:8500"},

		//Type:      provider.ProviderTypeZookeeper,
		//Endpoints: []string{"localhost:2181"},

		Type:      provider.ProviderTypeNacos,
		Endpoints: []string{"localhost:8848"},
		Username:  "nacos",
		Password:  "nacos",
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

	p.SetOutputLogCallback(func(logType dynaconfig.OutputLogType, message string) {
		if logType == dynaconfig.OutputLogTypeError {
			fmt.Fprintf(os.Stderr, "ERROR - "+message+"\n")
		} else if logType == dynaconfig.OutputLogTypeWarn {
			fmt.Fprintf(os.Stdout, "WARN - "+message+"\n")
		} else if logType == dynaconfig.OutputLogTypeDebug {
			fmt.Fprintf(os.Stdout, "DEBUG - "+message+"\n")
		} else {
			fmt.Fprintf(os.Stdout, "INFO -"+message+"\n")
		}
	})

	manager := dynaconfig.NewConfigManager(p, configs, keys)
	manager.SetOutputLogCallback(func(logType dynaconfig.OutputLogType, message string) {
		if logType == dynaconfig.OutputLogTypeError {
			fmt.Fprintf(os.Stderr, "ERROR - "+message+"\n")
		} else if logType == dynaconfig.OutputLogTypeWarn {
			fmt.Fprintf(os.Stdout, "WARN - "+message+"\n")
		} else if logType == dynaconfig.OutputLogTypeDebug {
			fmt.Fprintf(os.Stdout, "DEBUG - "+message+"\n")
		} else {
			fmt.Fprintf(os.Stdout, "INFO -"+message+"\n")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.ASyncConfig(ctx)
	//if err = manager.SyncConfig(context.Background()); err != nil {
	//	fmt.Fprintf(os.Stderr, "Failed to sync config: %v\n", err)
	//	return
	//}

	if err = manager.Watch(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start watch: %v \n", err)
		return
	}

	defer func() {
		if err = manager.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close: %v \n", err)
		}
	}()

	//////////////////////// testing /////////////////////////
	// Testing read the configuration content in real time
	go func() {
		for {
			time.Sleep(3 * time.Second)
			for _, key := range keys {
				config := manager.GetLocalConfig(key)
				fmt.Printf("+++++++ >>> Current config for %s: %+v\n", key, config)
			}
		}
	}()
	/////////////////////////////////////////////////

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit...")
	<-sigCh

	fmt.Println("\nReceived shutdown signal. Exiting...")
	os.Exit(0)

}
