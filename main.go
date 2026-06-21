package main

import "log"

func main() {
	registry := NewVirtualNamespaceRegistry("./data/namespace-registry.json")

	// Start the Admin API on port 8089
	go startAdminServer(8089, registry)

	proxy, err := NewStickyProxy("localhost:8088", registry)
	if err != nil {
		log.Fatal("Error creating proxy: ", err)
	}
	proxy.Start()
}
