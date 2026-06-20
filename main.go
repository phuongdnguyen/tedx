package main

import "log"

func main() {
	virtualNamespace := NewVirtualNamespace("default")
	physNamespaces := []string{
		buildPhysicalNamespace("default-1", "localhost:7233"),
		buildPhysicalNamespace("default-2", "localhost:7234"),
	}
	for _, physNamespace := range physNamespaces {
		virtualNamespace.Add(&Namespace{
			name: physNamespace,
		})
	}
	registry := NewVirtualNamespaceRegistry([]*VirtualNamespace{virtualNamespace})
	proxy, err := NewTemporalProxy("localhost:8088", registry)
	if err != nil {
		log.Fatal("Error creating proxy: ", err)
	}
	proxy.Start()
}
