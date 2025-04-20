package instance

import "time"

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
