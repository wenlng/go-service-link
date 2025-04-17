package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wenlng/service-discovery/golang/load_balancer"
	"github.com/wenlng/service-discovery/golang/service_discovery"
)

func Test_RegisterService(t *testing.T) {
	serviceName := "hello-app"
	discovery, err := service_discovery.NewDiscoveryWithLB(service_discovery.Config{
		Type:        service_discovery.ServiceDiscoveryTypeEtcd,
		Addrs:       "localhost:2379",
		TTL:         10 * time.Second,
		KeepAlive:   3 * time.Second,
		ServiceName: serviceName,
	}, load_balancer.LoadBalancerTypeRoundRobin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize service discovery: %v\n", err)
	}

	discovery.SetLogOutputHookFunc(func(logType service_discovery.ServiceDiscoveryLogType, message string) {
		if logType == service_discovery.ServiceDiscoveryLogTypeInfo {
			fmt.Fprintf(os.Stdout, "[Service Discovery Log]: %v\n", message)
		} else {
			fmt.Fprintf(os.Stderr, "[Service Discovery Log]: %v\n", message)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register service
	instanceID := uuid.New().String()
	addr := "localhost:8080"
	fmt.Fprintf(os.Stderr, "hello")
	if err = discovery.Register(ctx, serviceName, instanceID, addr, "8080", "0"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register service:: %v\n", err)
	}
	go updateInstances(ctx, discovery, serviceName, instanceID)

	// Close
	defer func() {
		if err = discovery.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Service discovery close error: %v\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "Service discovery closed successfully\n")
		}
	}()

	time.Sleep(time.Second * 10)
}

// updateInstances periodically updates service instances
func updateInstances(ctx context.Context, discovery service_discovery.ServiceDiscovery, serviceName, instanceID string) {
	if discovery != nil {
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
		case instances := <-ch:
			fmt.Fprintf(os.Stdout, "Discovered instances: %d \n", len(instances))
		}
	}
}
