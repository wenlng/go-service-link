package main

import (
	"fmt"
	"os"
	"time"

	"github.com/wenlng/go-service-discovery/helper"
	"github.com/wenlng/go-service-discovery/loadbalancer"
	"github.com/wenlng/go-service-discovery/servicediscovery"
)

var discovery *servicediscovery.DiscoveryWithLB

// setupDiscovery .
func setupDiscovery(serviceName string) error {
	var err error
	discovery, err = servicediscovery.NewDiscoveryWithLB(servicediscovery.Config{
		//Type:        servicediscovery.ServiceDiscoveryTypeEtcd,
		//Addrs:       "localhost:2379",

		//Type:        servicediscovery.ServiceDiscoveryTypeZookeeper,
		//Addrs:       "localhost:2181",

		//Type:        servicediscovery.ServiceDiscoveryTypeNacos,
		//Addrs:       "localhost:8848",
		//Username: "nocos",
		//Password: "nocos",

		Type:  servicediscovery.ServiceDiscoveryTypeConsul,
		Addrs: "localhost:8500",

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
