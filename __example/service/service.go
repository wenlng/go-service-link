package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/wenlng/go-captcha-discovery/servicediscovery"
)

var discovery servicediscovery.ServiceDiscovery

// setupDiscovery .
func setupDiscovery(serviceName, httPort, grpcPort string) error {
	var err error
	discovery, err = servicediscovery.NewServiceDiscovery(servicediscovery.Config{
		//Type:        servicediscovery.ServiceDiscoveryTypeEtcd,
		//Addrs:       "localhost:2379",

		//Type:        servicediscovery.ServiceDiscoveryTypeZookeeper,
		//Addrs:       "localhost:2181",

		//Type:        servicediscovery.ServiceDiscoveryTypeNacos,
		//Addrs:       "localhost:8848",
		//Username: "nocos",
		//Password: "nocos",

		Type:        servicediscovery.ServiceDiscoveryTypeConsul,
		Addrs:       "localhost:8500",
		TTL:         10 * time.Second,
		KeepAlive:   3 * time.Second,
		ServiceName: serviceName,
	})
	if err != nil {
		return err
	}

	discovery.SetLogOutputHookFunc(func(logType servicediscovery.ServiceDiscoveryLogType, message string) {
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

	time.Sleep(time.Second * 100000)
}

// watchInstances periodically updates service instances
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
