package base

// ServiceInstance represents a service instance
type ServiceInstance struct {
	InstanceID string
	Host       string
	HTTPPort   string
	GRPCPort   string
	Metadata   map[string]string
}
