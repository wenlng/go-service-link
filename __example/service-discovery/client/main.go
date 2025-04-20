package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/wenlng/go-service-link/servicediscovery"
	"github.com/wenlng/go-service-link/servicediscovery/balancer"
)

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
