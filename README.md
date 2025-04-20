<div align="center">
<h1 style="margin: 0; padding: 0">GoServiceLink</h1>
<p style="margin: 0; padding: 0">Service Discovery and Dynamic Configuration Management for Golang</p>
<br/>
<a href="https://goreportcard.com/report/github.com/wenlng/go-service-link"><img src="https://goreportcard.com/badge/github.com/wenlng/go-service-link"/></a>
<a href="https://godoc.org/github.com/wenlng/go-service-link"><img src="https://godoc.org/github.com/wenlng/go-service-link?status.svg"/></a>
<a href="https://github.com/wenlng/go-service-link/releases"><img src="https://img.shields.io/github/v/release/wenlng/go-service-link.svg"/></a>
<a href="https://github.com/wenlng/go-service-link/blob/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg"/></a>
<a href="https://github.com/wenlng/go-service-link"><img src="https://img.shields.io/github/stars/wenlng/go-service-link.svg"/></a>
<a href="https://github.com/wenlng/go-service-link"><img src="https://img.shields.io/github/last-commit/wenlng/go-service-link.svg"/></a>
</div>

<br/>

`GoServiceLink` s a manager for service discovery and dynamic configuration, providing a collection of adaptable middleware for microservice architectures. It supports service registration and discovery, load balancing, dynamic configuration synchronization, real-time monitoring, and hot reloading, along with powerful features like connection pooling, health checks, and exponential backoff retries.

> English | [中文](README_zh.md)

⭐️ If this project helps you, please give it a star!

## Features
- **Service Registration**: Automatically registers service information with middleware upon startup, supporting lease mechanisms to ensure service state updates.
- **Service Discovery**: Clients dynamically retrieve service lists from middleware, with real-time monitoring of service changes.
- **Load Balancing**: Offers multiple load balancing strategies (Random, Round-Robin, Consistent-Hash).
- **Supported Middleware**: Etcd, Consul, ZooKeeper, Nacos.
- **Dynamic Configuration Management**: Synchronizes local and remote configurations, monitors configuration changes in real-time, and triggers hot reloading.
- **Connection Pool**: Maintains client connection pools for each configuration center to improve performance and resource utilization.
- **Distributed Locks**: Uses distributed locks (e.g., Etcd mutex, Consul lock, ZooKeeper lock) to ensure the safety of configuration updates.
- **Health Checks**: Periodically checks the health status of configuration centers, providing metrics like latency, leader status, and cluster size.
- **Reconnection and Retry**: Automatically reconnects to disconnected configuration centers and retries failed operations using an exponential backoff strategy.
- **Graceful Shutdown**: Properly cleans up resources (connections, listeners) when the application terminates.
- **Example Code**: Includes server/client examples demonstrating practical applications of service discovery.
- **Modular Design**: Clear code structure (servicediscovery, dynaconfig), easy to extend and integrate.


## Server Side

Below is an example of server-side service registration code:

```go
var discovery servicediscovery.ServiceDiscovery

// setupDiscovery .
func setupDiscovery(serviceName, httPort, grpcPort string) error {
	var err error
	discovery, err = servicediscovery.NewServiceDiscovery(servicediscovery.Config{
		//Type:  servicediscovery.ServiceDiscoveryTypeEtcd,
		//Addrs: "localhost:2379",

		//Type:  servicediscovery.ServiceDiscoveryTypeConsul,
		//Addrs: "localhost:8500",

		//Type:  servicediscovery.ServiceDiscoveryTypeZookeeper,
		//Addrs: "localhost:2181",

		Type:     servicediscovery.ServiceDiscoveryTypeNacos,
		Addrs:    "localhost:8848",
		Username: "nacos",
		Password: "nacos",

		ServiceName: serviceName,
	})
	if err != nil {
		return err
	}

	discovery.SetOutputLogCallback(func(logType servicediscovery.OutputLogType, message string) {
		if logType == servicediscovery.OutputLogTypeError {
			fmt.Fprintf(os.Stderr, "ERROR - "+message+"\n")
		} else if logType == servicediscovery.OutputLogTypeWarn {
			fmt.Fprintf(os.Stdout, "WARN - "+message+"\n")
		} else if logType == servicediscovery.OutputLogTypeDebug {
			fmt.Fprintf(os.Stdout, "DEBUG - "+message+"\n")
		} else {
			fmt.Fprintf(os.Stdout, "INFO -"+message+"\n")
		}
	})

	return nil
}

func main() {
	httpPort := flag.String("http-port", "8001", "Port for HTTP server")
	grpcPort := flag.String("grpc-port", "9001", "Port for gRPC server")
	flag.Parse()

	serviceName := "hello-app"
	host := "localhost"

	err := setupDiscovery(serviceName, *httpPort, *grpcPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize service discovery: %v\n", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register service
	instanceID := uuid.New().String()
	if err = discovery.Register(ctx, serviceName, instanceID, host, *httpPort, *grpcPort); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register service: %v\n", err)
	}

	// Watch service
	go watchInstances(ctx, discovery, serviceName, instanceID)

	// Close
	defer func() {
		if err = discovery.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Service discovery close error: %v\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "Service discovery closed successfully\n")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit...")
	<-sigCh

	fmt.Println("\nReceived shutdown signal. Exiting...")
	os.Exit(0)
}

// watchInstances ...
func watchInstances(ctx context.Context, discovery servicediscovery.ServiceDiscovery, serviceName, instanceID string) {
	if discovery == nil {
		return
	}

	ch, err := discovery.Watch(ctx, serviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discovery watch: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			if discovery != nil {
				if err = discovery.Deregister(ctx, serviceName, instanceID); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to deregister service: %v\n", err)
				}
			}
			return
		case instances, ok := <-ch:
			if !ok {
				return
			}
			instancesStr, _ := json.Marshal(instances)
			fmt.Fprintf(os.Stdout, "Discovered instances: %d, list: %v \n", len(instances), string(instancesStr))
		}
	}
}
```

## Client Side

Below is an example of client-side service discovery and load balancing code:

```go
var discovery *servicediscovery.DiscoveryWithLB

// setupDiscovery .
func setupDiscovery(serviceName string) error {
	var err error
	discovery, err = servicediscovery.NewDiscoveryWithLB(servicediscovery.Config{
		//Type:  servicediscovery.ServiceDiscoveryTypeEtcd,
		//Addrs: "localhost:2379",

		//Type:  servicediscovery.ServiceDiscoveryTypeConsul,
		//Addrs: "localhost:8500",

		//Type:  servicediscovery.ServiceDiscoveryTypeZookeeper,
		//Addrs: "localhost:2181",

		Type:     servicediscovery.ServiceDiscoveryTypeNacos,
		Addrs:    "localhost:8848",
		Username: "nacos",
		Password: "nacos",

		ServiceName: serviceName,
	}, balancer.LoadBalancerTypeRoundRobin)
	if err != nil {
		return err
	}

	discovery.SetOutputLogCallback(func(logType servicediscovery.OutputLogType, message string) {
		if logType == servicediscovery.OutputLogTypeError {
			fmt.Fprintf(os.Stderr, "ERROR - "+message+"\n")
		} else if logType == servicediscovery.OutputLogTypeWarn {
			fmt.Fprintf(os.Stdout, "WARN - "+message+"\n")
		} else if logType == servicediscovery.OutputLogTypeDebug {
			fmt.Fprintf(os.Stdout, "DEBUG - "+message+"\n")
		} else {
			fmt.Fprintf(os.Stdout, "INFO -"+message+"\n")
		}
	})

	return nil
}

// watchInstances ...
func watchInstances(ctx context.Context, discovery servicediscovery.ServiceDiscovery, serviceName string) {
	if discovery == nil {
		return
	}

	ch, err := discovery.Watch(ctx, serviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discovery watch: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case instances, ok := <-ch:
			if !ok {
				return
			}
			instancesStr, _ := json.Marshal(instances)
			fmt.Fprintf(os.Stdout, "Discovered instances: %d, list: %v \n", len(instances), string(instancesStr))
		}
	}
}

func selectUrl(serviceName string) string {
	hostname, _ := os.Hostname()
	inst, err := discovery.Select(serviceName, hostname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to select instance: %v\n", err)
		return ""
	}
	httpPort := inst.HTTPPort
	return fmt.Sprintf("http://%s:%s/hello", inst.Host, httpPort)
}

func callRequests(serviceName string, numWorkers, requestsPerWorker int) {
	wg := sync.WaitGroup{}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				url := selectUrl(serviceName)
				fmt.Fprintf(os.Stdout, "worker: %d, request: %d selectUrl: %v\n", workerID, j, url)
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
}

func main() {
	numWorkers := flag.Int("worker", 5, "Port for HTTP server")
	requestsPerWorker := flag.Int("request", 10, "Port for gRPC server")
	flag.Parse()

	serviceName := "hello-app"
	err := setupDiscovery(serviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize service discovery: %v\n", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchInstances(ctx, discovery, serviceName)

	// Close
	defer func() {
		if err = discovery.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Service discovery close error: %v\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "Service discovery closed successfully\n")
		}
	}()

	fmt.Println(">>>>>>> string call request ...")
	go func() {
		for {
			callRequests(serviceName, *numWorkers, *requestsPerWorker)
			time.Sleep(1 * time.Second)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit...")
	<-sigCh

	fmt.Println("\nReceived shutdown signal. Exiting...")
	os.Exit(0)
}
```

<br/>
<br/>
<hr/>

## Dynamic Configuration Management
> Synchronization Mechanism: Automatically updates based on version control. When the configuration center’s version is higher than the local version, it syncs to the local system; otherwise, it syncs to the configuration center.

Below is an example of dynamic configuration usage:

```go
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
```

## LICENSE

MIT