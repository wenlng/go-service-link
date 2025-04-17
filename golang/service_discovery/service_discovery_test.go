package service_discovery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestEtcdDiscovery(t *testing.T) {
	discovery, err := NewEtcdDiscovery("localhost:2379", time.Second*10, time.Second*3)
	if err != nil {
		t.Skip("etcd not available")
	}
	defer discovery.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serviceName := "test-service"
	instanceID := uuid.New().String()
	addr := "localhost:8080"
	httpPort := "8080"
	grpcPort := "50051"

	t.Run("Register", func(t *testing.T) {
		err := discovery.Register(ctx, serviceName, instanceID, addr, httpPort, grpcPort)
		assert.NoError(t, err)

		instances, err := discovery.GetInstances(serviceName)
		assert.NoError(t, err)
		assert.Len(t, instances, 1)
		assert.Equal(t, instanceID, instances[0].InstanceID)
		assert.Equal(t, addr, instances[0].Addr)
		assert.Equal(t, httpPort, instances[0].Metadata["http_port"])
		assert.Equal(t, grpcPort, instances[0].Metadata["grpc_port"])
	})

	t.Run("Watch", func(t *testing.T) {
		ch, err := discovery.Watch(ctx, serviceName)
		assert.NoError(t, err)

		select {
		case instances := <-ch:
			assert.Len(t, instances, 1)
			assert.Equal(t, instanceID, instances[0].InstanceID)
		case <-time.After(time.Second * 5):
			t.Fatal("Watch timeout")
		}
	})

	t.Run("Deregister", func(t *testing.T) {
		err := discovery.Deregister(ctx, serviceName, instanceID)
		assert.NoError(t, err)

		instances, err := discovery.GetInstances(serviceName)
		assert.NoError(t, err)
		assert.Len(t, instances, 0)
	})
}

func TestDiscoveryWithLB(t *testing.T) {
	config := Config{
		Type:        "etcd",
		Addrs:       "localhost:2379",
		TTL:         time.Second * 10,
		KeepAlive:   time.Second * 3,
		ServiceName: "test-service",
	}
	dlb, err := NewDiscoveryWithLB(config, "round_robin")
	if err != nil {
		t.Skip("etcd not available")
	}
	defer dlb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serviceName := "test-service"
	instanceID1 := uuid.New().String()
	instanceID2 := uuid.New().String()
	addr1 := "localhost:8081"
	addr2 := "localhost:8082"
	httpPort := "8080"
	grpcPort := "50051"

	t.Run("RegisterAndSelect", func(t *testing.T) {
		err := dlb.Register(ctx, serviceName, instanceID1, addr1, httpPort, grpcPort)
		assert.NoError(t, err)
		err = dlb.Register(ctx, serviceName, instanceID2, addr2, httpPort, grpcPort)
		assert.NoError(t, err)

		for i := 0; i < 4; i++ {
			inst, err := dlb.Select(serviceName, fmt.Sprintf("key%d", i))
			assert.NoError(t, err)
			assert.Contains(t, []string{addr1, addr2}, inst.Addr)
			assert.Equal(t, httpPort, inst.Metadata["http_port"])
			assert.Equal(t, grpcPort, inst.Metadata["grpc_port"])
		}
	})

	t.Run("Watch", func(t *testing.T) {
		ch, err := dlb.Watch(ctx, serviceName)
		assert.NoError(t, err)

		select {
		case instances := <-ch:
			assert.Len(t, instances, 2)
			addrs := []string{instances[0].Addr, instances[1].Addr}
			assert.Contains(t, addrs, addr1)
			assert.Contains(t, addrs, addr2)
		case <-time.After(time.Second * 5):
			t.Fatal("Watch timeout")
		}
	})

	t.Run("Deregister", func(t *testing.T) {
		err := dlb.Deregister(ctx, serviceName, instanceID1)
		assert.NoError(t, err)

		instances, err := dlb.GetInstances(serviceName)
		assert.NoError(t, err)
		assert.Len(t, instances, 1)
		assert.Equal(t, instanceID2, instances[0].InstanceID)
	})
}
