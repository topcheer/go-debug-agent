package debugagent

import (
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

func registerNetworkInspector() {
	RegisterTool("get_network_stats", "Get network-related info: local addresses, hostname, active goroutines, connection overview", nil, func(args map[string]any) (any, error) {
		return map[string]any{
			"goroutines":      runtime.NumGoroutine(),
			"hostname":        getHostname(),
			"local_addresses": getLocalAddresses(),
			"interfaces":      getInterfaceInfo(),
		}, nil
	})

	RegisterTool("get_dns_info", "Get DNS resolver info and test DNS resolution for a hostname", map[string]ToolParam{
		"hostname": {Type: "string", Description: "Hostname to resolve (default: localhost)", Required: false},
	}, func(args map[string]any) (any, error) {
		hostname := "localhost"
		if h, ok := args["hostname"].(string); ok && h != "" {
			hostname = h
		}

		start := time.Now()
		ips, err := net.LookupIP(hostname)
		resolutionTime := time.Since(start)

		result := map[string]any{
			"hostname":        hostname,
			"resolution_time": resolutionTime.String(),
			"nameserver":      getNameserver(),
		}

		if err != nil {
			result["error"] = err.Error()
		} else {
			ipStrs := make([]string, 0, len(ips))
			for _, ip := range ips {
				ipStrs = append(ipStrs, ip.String())
			}
			result["resolved_ips"] = ipStrs
		}

		return result, nil
	})
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}

func getLocalAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []string{}
	}
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr.String())
	}
	return result
}

func getInterfaceInfo() []map[string]any {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	result := make([]map[string]any, 0, len(ifaces))
	for _, iface := range ifaces {
		entry := map[string]any{
			"name":  iface.Name,
			"index": iface.Index,
			"mtu":   iface.MTU,
			"flags": iface.Flags.String(),
		}
		addrs, err := iface.Addrs()
		if err == nil {
			addrStrs := make([]string, 0, len(addrs))
			for _, a := range addrs {
				addrStrs = append(addrStrs, a.String())
			}
			entry["addresses"] = addrStrs
		}
		result = append(result, entry)
	}
	return result
}

func getNameserver() string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			return strings.TrimSpace(line[len("nameserver "):])
		}
	}
	return "default"
}
