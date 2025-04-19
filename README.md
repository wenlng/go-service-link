<div align="center">
<h1 style="margin: 0; padding: 0">GoServiceDiscovery</h1>
<p style="margin: 0; padding: 0">The Service Discovery for the Golang</p>
<br/>
<a href="https://goreportcard.com/report/github.com/wenlng/go-service-discovery"><img src="https://goreportcard.com/badge/github.com/wenlng/go-service-discovery"/></a>
<a href="https://godoc.org/github.com/wenlng/go-service-discovery"><img src="https://godoc.org/github.com/wenlng/go-service-discovery?status.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery/releases"><img src="https://img.shields.io/github/v/release/wenlng/go-service-discovery.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery/blob/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery"><img src="https://img.shields.io/github/stars/wenlng/go-service-discovery.svg"/></a>
<a href="https://github.com/wenlng/go-service-discovery"><img src="https://img.shields.io/github/last-commit/wenlng/go-service-discovery.svg"/></a>
</div>

<br/>

`GoServiceDiscovery` is a convenient library for integrating service discovery, supporting multiple middleware, suitable for microservice architectures, and providing service registration, service discovery, and load balancing functionalities.

> English | [中文](README_zh.md)

⭐️ If this project helps you, please give it a star!

## Features

- **Service Registration**: Automatically registers service information with the service middleware upon startup, supporting lease mechanisms to ensure service status updates.
- **Service Discovery**: Clients dynamically retrieve service instance lists from the service middleware, supporting real-time monitoring of service changes.
- **Load Balancing**: Provides multiple load balancing strategies (Random, Round-Robin, Consistent-Hash).
- **Example Code**: Includes server and client examples demonstrating practical applications of service discovery.
- **Modular Design**: Clear code structure, easy to extend and integrate.
- **Supported Middleware**: Etcd, Consul, ZooKeeper, Nacos.

## Server Side

Below is an example of server-side service registration code:

```go
var discovery servicediscovery.ServiceDiscovery

// setupDiscovery configures service discovery
func setupDiscovery(serviceName, httpPort, grpcPort string) error {
    var err error
    discovery, err = servicediscovery.NewServiceDiscovery(servicediscovery.Config{
        Type:        servicediscovery.ServiceDiscoveryTypeEtcd,
        Addrs:       "localhost:2379",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeZookeeper,
        //Addrs:       "localhost:2181",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeNacos,
        //Addrs:       "localhost:8848",
        //Username:    "nacos",
        //Password:    "nacos",
        
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

// watchInstances monitors service instances
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

## Client Side

Below is an example of client-side service discovery and load balancing code:

```go
var discovery *servicediscovery.DiscoveryWithLB

// setupDiscovery configures service discovery
func setupDiscovery(serviceName string) error {
    var err error
    discovery, err = servicediscovery.NewDiscoveryWithLB(servicediscovery.Config{
        Type:        servicediscovery.ServiceDiscoveryTypeEtcd,
        Addrs:       "localhost:2379",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeZookeeper,
        //Addrs:       "localhost:2181",
        
        //Type:        servicediscovery.ServiceDiscoveryTypeNacos,
        //Addrs:       "localhost:8848",
        //Username:    "nacos",
        //Password:    "nacos",
        
        // Type:        servicediscovery.ServiceDiscoveryTypeConsul,
        // Addrs:       "localhost:8500",
        
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
        fmt.Fprintf(os.Stderr, "Failed to select instance: %v\n", err)
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

## LICENSE

MIT