package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/wenlng/go-service-link/servicediscovery"
)

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
