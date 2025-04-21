package instance

import (
	"fmt"
	"time"
)

// ServiceInstance represents a service instance
type ServiceInstance struct {
	InstanceID string
	Host       string
	HTTPPort   string
	GRPCPort   string
	Metadata   map[string]string

	IsHealthy    bool
	LastChecked  time.Time
	FailedChecks int
}

// GetHost .
func (si *ServiceInstance) GetHost() string {
	if si.Host != "" {
		return si.Host
	}
	return "127.0.0.1"
}

// GetHTTPPort .
func (si *ServiceInstance) GetHTTPPort() string {
	if si.HTTPPort != "" {
		return si.HTTPPort
	}

	if port, ok := si.Metadata["http_port"]; ok {
		return port
	}

	return "80"
}

// GetGRPCPort .
func (si *ServiceInstance) GetGRPCPort() string {
	if si.GRPCPort != "" {
		return si.GRPCPort
	}

	if port, ok := si.Metadata["grpc_port"]; ok {
		return port
	}

	return "50051"
}

// GetHTTPAddress .
func (si *ServiceInstance) GetHTTPAddress() string {
	return fmt.Sprintf("%s:%s", si.GetHost(), si.GetHTTPPort())
}

// GetGRPCAddress .
func (si *ServiceInstance) GetGRPCAddress() string {
	return fmt.Sprintf("%s:%s", si.GetHost(), si.GetGRPCPort())
}
