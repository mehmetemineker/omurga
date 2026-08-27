package gateway

import (
	"fmt"
	"hash/crc32"
	"sort"
)

const (
	PortStart = 20000
	PortEnd   = 29999
)

type Target struct {
	Service       string
	ContainerPort int
}

func EnvironmentKey(environment string) string {
	if environment == "" {
		return "default"
	}
	return environment
}

func Key(service string, containerPort int) string {
	return fmt.Sprintf("%s:%d", service, containerPort)
}

func Candidate(project, environment string, target Target) int {
	seed := project + ":" + EnvironmentKey(environment) + ":" + Key(target.Service, target.ContainerPort)
	rangeSize := PortEnd - PortStart + 1
	return PortStart + int(crc32.ChecksumIEEE([]byte(seed))%uint32(rangeSize))
}

func UniqueTargets(targets []Target) ([]Target, error) {
	unique := make(map[string]Target, len(targets))
	for _, target := range targets {
		if target.Service == "" {
			return nil, fmt.Errorf("gateway target service is required")
		}
		if target.ContainerPort < 1 || target.ContainerPort > 65535 {
			return nil, fmt.Errorf("gateway target port for service %s must be between 1 and 65535", target.Service)
		}
		unique[Key(target.Service, target.ContainerPort)] = target
	}

	result := make([]Target, 0, len(unique))
	for _, target := range unique {
		result = append(result, target)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Service == result[j].Service {
			return result[i].ContainerPort < result[j].ContainerPort
		}
		return result[i].Service < result[j].Service
	})
	return result, nil
}

func NextAvailable(candidate int, used map[int]bool) (int, error) {
	if candidate < PortStart || candidate > PortEnd {
		return 0, fmt.Errorf("gateway port candidate %d is outside the managed range", candidate)
	}
	port := candidate
	for attempts := 0; attempts <= PortEnd-PortStart; attempts++ {
		if !used[port] {
			return port, nil
		}
		port++
		if port > PortEnd {
			port = PortStart
		}
	}
	return 0, fmt.Errorf("gateway port range %d-%d is exhausted", PortStart, PortEnd)
}
