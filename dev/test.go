package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

func main() {
	client, err := clients.NewNamingClient(vo.NacosClientParam{
		ClientConfig: &constant.ClientConfig{
			NamespaceId: "",
			Username:    "nacos",
			Password:    "nacos",
		},
		ServerConfigs: []constant.ServerConfig{
			{IpAddr: "localhost", Port: 8848},
		},
	})
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		return
	}

	success, err := client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          "127.0.0.1",
		Port:        8080,
		ServiceName: "hello-app",
		Weight:      1,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
	})
	fmt.Printf("RegisterInstance: success=%v, err=%v\n", success, err)

	subscribeParam := &vo.SubscribeParam{
		ServiceName: "hello-app",
		SubscribeCallback: func(services []model.Instance, err error) {
			fmt.Println("services >>>>>>>>>>>", services)
		},
	}

	err = client.Subscribe(subscribeParam)
	if err != nil {
		fmt.Println("sub err >>>>>>>>>>>", err)
	}

	defer func() {
		_ = client.Unsubscribe(subscribeParam)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to exit...")
	<-sigCh

	fmt.Println("\nReceived shutdown signal. Exiting...")
	os.Exit(0)
}
