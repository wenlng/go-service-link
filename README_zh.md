<div align="center">
<h1 style="margin: 0; padding: 0">GoServiceLink</h1>
<p style="margin: 0; padding: 0">用于 Golang 的服务发现和服务动态配置管理 </p>
<br/>
<a href="https://goreportcard.com/report/github.com/wenlng/go-service-link"><img src="https://goreportcard.com/badge/github.com/wenlng/go-service-link"/></a>
<a href="https://godoc.org/github.com/wenlng/go-service-link"><img src="https://godoc.org/github.com/wenlng/go-service-link?status.svg"/></a>
<a href="https://github.com/wenlng/go-service-link/releases"><img src="https://img.shields.io/github/v/release/wenlng/go-service-link.svg"/></a>
<a href="https://github.com/wenlng/go-service-link/blob/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg"/></a>
<a href="https://github.com/wenlng/go-service-link"><img src="https://img.shields.io/github/stars/wenlng/go-service-link.svg"/></a>
<a href="https://github.com/wenlng/go-service-link"><img src="https://img.shields.io/github/last-commit/wenlng/go-service-link.svg"/></a>
</div>

<br/>

`GoServiceLink` 是服务发现和动态配置的管理器，提供多种中间件可适配的合集，适用于微服务架构，支持服务注册与发现、负载均衡、动态配置同步、实时监控和热加载等功能，同时具备连接池、健康检查和指数退避重试等强大功能。

<br/>

> [English](README.md) | 中文
<p> ⭐️ 如果能帮助到你，请随手给点一个star</p>

## 功能特性
- **服务注册**：服务启动时自动向服务中间件注册服务信息，支持租约机制确保服务状态更新。
- **服务发现**：客户端从服务中间件动态获取服务列表，支持实时监听服务变化。
- **负载均衡**：提供轮询多种负载均衡策略（Random、Round-Robin、Consistent-Hash）。
- **支持中间件**：Etcd、Consul、ZooKeeper、Nacos。
- **动态配置管理**： 同步本地和远程配置、实时监控配置变化并触发热加载。
- **连接池**：为每个配置中心维护客户端连接池，提升性能和资源利用率。
- **分布式锁**：使用分布式锁（例如 Etcd mutex、Consul 锁、ZooKeeper 锁）确保配置更新的安全性。
- **健康检查**：定期检查配置中心的健康状态，提供延迟、领导者状态和集群规模等指标。
- **重连和重试**： 自动重连断开的配置中心、使用指数退避策略重试失败的操作。
- **优雅关闭**：在应用终止时正确清理资源（连接、监听器）。
- **示例代码**：包含服务端/客户端示例，展示服务发现的实际应用。
- **模块化设计**：代码结构清晰（servicediscovery、dynaconfig），易于扩展和集成。

<br/>

## 服务发现 - Service 端
下面是一个服务器端服务注册代码的例子：

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



## 服务发现 - Client 端
下面是客户端服务发现和负载平衡代码的示例：
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


## 服务动态配置管理

> 同步机制：自动根据 Version 版本控制更新，当配置中心的版本号大于本地配置版本时，会自动同步到本地；反之则会同步到配置中心。

下面是服务动态配置使用的代码的示例：

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

	manager, err := dynaconfig.NewConfigManager(dynaconfig.ConfigManagerParams{
		ProviderConfig: providerCfg,
		Configs:        configs,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create config mananger, err: %v \n", err)
	}
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
			if helper.IsOnlyEmpty(config.Content) {
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
			for _, key := range []string{"/config/my-app/main", "/config/my-app/db"} {
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

<br/>