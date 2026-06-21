package config

import (
	"encoding/json"
	"os"

	"github.com/docker/go-units"
)

type PodConfig struct {
	ReservedPorts []int  `json:"reserved_ports"`
	CPULimit      string `json:"cpu_limit"`    // e.g. "2" or "0.5"
	MemoryLimit   string `json:"memory_limit"` // e.g. "512MB" or "1GB"
}

type Config struct {
	Pods map[string]PodConfig `json:"pods"` // map of Pod Token to PodConfig
}

func LoadGuardConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// MemoryBytes parses a string like "512MB" to int64 bytes using go-units.
func (p *PodConfig) MemoryBytes() (int64, error) {
	if p.MemoryLimit == "" {
		return 0, nil
	}
	return units.RAMInBytes(p.MemoryLimit)
}
