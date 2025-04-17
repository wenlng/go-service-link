package types

// Instance represents a service instance
type Instance struct {
	InstanceID string
	Addr       string
	HTTPPort   int
	GRPCPort   int
	Metadata   map[string]string
}
