package helper

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// GetHostname .
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "-"
	}
	return hostname
}

// SplitHostPort .
func SplitHostPort(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", addr)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", parts[1])
	}
	return parts[0], port, nil
}

// IsDurationSet .
func IsDurationSet(d time.Duration) bool {
	return d != 0
}
