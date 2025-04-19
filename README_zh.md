<div align="center">
<h1 style="margin: 0; padding: 0">GoServiceDiscovery</h1>
<p style="margin: 0; padding: 0">用于 Golang 的服务发现 + 动态配置管理器 </p>
<br/>
<a href="https://goreportcard.com/report/github.com/wenlng/go-service-discovery"><img src="https://goreportcard.com/badge/github.com/wenlng/go-service-discovery"/></a>
<a href="https://godoc.org/github.com/wenlng/go-service-discovery"><img src="https://godoc.org/github.com/wenlng/go-service-discovery?status.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery/releases"><img src="https://img.shields.io/github/v/release/wenlng/go-service-discovery.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery/blob/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery"><img src="https://img.shields.io/github/stars/wenlng/go-service-discovery.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery"><img src="https://img.shields.io/github/last-commit/wenlng/go-service-discovery.svg"/></a>
</div>

<br/>

`ServiceDiscovery` 是服务发现管理器，多种中间件可适配支持的合集，适用于微服务架构，支持服务注册、服务发现和负载均衡功能，同时具备连接池、健康检查和指数退避重试等强大功能。
<br/>

`DynamiConfig` 是动态配置管理器，旨在跨多个配置中心动态管理配置，提供统一的接口，支持配置同步、实时监控和热加载，同时具备连接池、分布式锁、健康检查和指数退避重试等强大功能。
<br/>
<br/>

> [English](README.md) | 中文
<p> ⭐️ 如果能帮助到你，请随手给点一个star</p>

## ServiceDiscovery 功能特性
* 服务注册：服务启动时自动向服务中间件注册服务信息，支持租约机制确保服务状态更新。
* 服务发现：客户端从服务中间件动态获取服务列表，支持实时监听服务变化。
* 负载均衡：提供轮询多种负载均衡策略（Random、Round-Robin、Consistent-Hash）。
* 示例代码：包含服务端/客户端示例，展示服务发现的实际应用。
* 模块化设计：代码结构清晰，易于扩展和集成。
* 支持中间件：Etcd、Consul、ZooKeeper、Nacos。


## DynamicConfig 特性

- **动态配置管理**：
    - 同步本地和远程配置。
    - 实时监控配置变化并触发热加载。
    - 验证配置有效性，确保数据正确性。
- **连接池**：为每个配置中心维护客户端连接池，提升性能和资源利用率。
- **分布式锁**：使用分布式锁（例如 Etcd mutex、Consul 锁、ZooKeeper 锁）确保配置更新的安全性。
- **健康检查**：定期检查配置中心的健康状态，提供延迟、领导者状态和集群规模等指标。
- **重连和重试**：
    - 自动重连断开的配置中心。
    - 使用指数退避策略重试失败的操作。
- **优雅关闭**：在应用终止时正确清理资源（连接、监听器）。

<br/>
<hr/>
<br/>

## Service 端
下面是一个服务器端服务注册代码的例子：
```go
var discovery servicediscovery.ServiceDiscovery

// setupDiscovery .
func setupDiscovery(serviceName, httPort, grpcPort string) error {
    var err error
    discovery, err = servicediscovery.NewServiceDiscovery(servicediscovery.Config{
        Type:        servicediscovery.ServiceDiscoveryTypeEtcd,
        Addrs:       "localhost:2379",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeZookeeper,
        //Addrs:       "localhost:2181",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeNacos,
        //Addrs:       "localhost:8848",
        //Username: "nocos",
        //Password: "nocos",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeConsul,
        //Addrs:       "localhost:8500",
        
        TTL:         10 * time.Second,
        KeepAlive:   3 * time.Second,
        ServiceName: serviceName,
    })
    if err != nil {
        return err
    }
    
    discovery.SetOutputLogCallback(func(logType servicediscovery.ServiceDiscoveryLogType, message string) {
        if logType == servicediscovery.ServiceDiscoveryLogTypeInfo {
            fmt.Fprintf(os.Stdout, "[Service Discovery Log]: %v\n", message)
        } else {
            fmt.Fprintf(os.Stderr, "[Service Discovery Log]: %v\n", message)
        }
    })
    
    return nil
}

func main() {
    serviceName := "hello-app"
    host := "localhost"
    httpPort := "8084"
    grpcPort := ""
    
    err := setupDiscovery(serviceName, httpPort, grpcPort)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to initialize service discovery: %v\n", err)
        return
    }
	
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Register service
    instanceID := uuid.New().String()
    if err = discovery.Register(ctx, serviceName, instanceID, host, httpPort, grpcPort); err != nil {
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
}


// watchInstances 监听服务实例
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
            fmt.Fprintf(os.Stdout, "Discovered instances: %d \n", len(instances))
        }
    }
}

```

## Client 端
下面是客户端服务发现和负载平衡代码的示例：
```go
var discovery *servicediscovery.DiscoveryWithLB

// setupDiscovery .
func setupDiscovery(serviceName string) error {
    var err error
    discovery, err = servicediscovery.NewDiscoveryWithLB(servicediscovery.Config{
        Type:        servicediscovery.ServiceDiscoveryTypeEtcd,
        Addrs:       "localhost:2379",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeZookeeper,
        //Addrs:       "localhost:2181",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeNacos,
        //Addrs:       "localhost:8848",
        //Username: "nocos",
        //Password: "nocos",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeConsul,
        //Addrs:       "localhost:8500",
        
        TTL:         10 * time.Second,
        KeepAlive:   3 * time.Second,
        ServiceName: serviceName,
    }, loadbalancer.LoadBalancerTypeConsistentHash)
    if err != nil {
        return err
    }

    discovery.SetOutputLogCallback(func(logType servicediscovery.ServiceDiscoveryLogType, message string) {
        if logType == servicediscovery.ServiceDiscoveryLogTypeInfo {
            fmt.Fprintf(os.Stdout, "[Service Discovery Log]: %v\n", message)
        } else {
            fmt.Fprintf(os.Stderr, "[Service Discovery Log]: %v\n", message)
        }
    })

    return nil
}

func main() {
    serviceName := "hello-app"
    
    err := setupDiscovery(serviceName)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to initialize service discovery: %v\n", err)
        return
    }
    
    // LB select
    inst, err := discovery.Select(serviceName, helper.GetHostname())
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to select instance: %v\n", err)
        return
    }
	
    httpPort, ok := inst.Metadata["http_port"]
    if !ok {
        fmt.Fprintf(os.Stderr, "http_port not found in instance metadata\n")
        return
    }
    url := fmt.Sprintf("http://%s:%s/hello", inst.Host, httpPort)
    fmt.Println(url)
    
    // Close
    defer func() {
        if err = discovery.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Service discovery close error: %v\n", err)
        } else {
            fmt.Fprintf(os.Stdout, "Service discovery closed successfully\n")
        }
    }()
}

```
<br/>
<br/>
<hr/>


## DynamicConfig 动态服务配置
下面是服务动态配置使用的代码的示例：
```go
func main() {
	configs := map[string]*provider.Config{
		"/config/go-captcha-service/app": {
			Name:    "app-config",
			Version: 0,
			//Version: 1856136267083311000,
			Content: `{"AppName": "my-app-main", "Port": 8081, DebugMode: false }`,
		},
		"/config/go-captcha-service/db": {
			Name:    "db-config",
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
			// testing panic ...
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
```

## LICENSE
MIT

<br/>