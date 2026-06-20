package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Namespace struct {
	cluster *Cluster
	name    string
}

type Cluster struct {
	address string
}

const delim = "::"

func buildPhysicalNamespace(actualName string, clusterAddress string) string {
	return strings.Join([]string{actualName, clusterAddress}, delim)
}

func parsePhysicalNamespace(physicalNamespace string) *Namespace {
	s := strings.Split(physicalNamespace, delim)
	return &Namespace{
		cluster: &Cluster{
			address: s[1],
		},
		name: s[0],
	}
}

// VirtualNamespaceRegistry allow lookup virtual namespace name to the actual VirtualNamespace object
type VirtualNamespaceRegistry struct {
	mu         sync.RWMutex
	namespaces map[string]*VirtualNamespace
}

// GetSize returns the number of virtual namespaces (for fallback logic)
func (vr *VirtualNamespaceRegistry) GetSize() int {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return len(vr.namespaces)
}

func NewVirtualNamespaceRegistry(dataPath string) *VirtualNamespaceRegistry {
	if dataPath == "" {
		dataPath = "./data/registry.json"
	}
	v := &VirtualNamespaceRegistry{
		namespaces: make(map[string]*VirtualNamespace),
	}
	
	// Read if exist
	file, err := os.ReadFile(dataPath)
	if err == nil {
		var schema map[string][]string
		if err := json.Unmarshal(file, &schema); err != nil {
			log.Fatalf("failed to unmarshal registry file %v, err: %v", dataPath, err)
		}
		for virtualName, slots := range schema {
			vNs := NewVirtualNamespace(virtualName)
			for _, slot := range slots {
				vNs.Add(&Namespace{name: slot})
			}
			v.namespaces[virtualName] = vNs
		}
		log.Printf("Loaded %d virtual namespaces from %s", len(v.namespaces), dataPath)
	} else if !os.IsNotExist(err) {
		log.Fatalf("failed to read registry file %v, err: %v", dataPath, err)
	}

	// Checkpointing routine
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			v.mu.RLock()
			schema := make(map[string][]string)
			for name, vNs := range v.namespaces {
				schema[name] = vNs.Hasher.GetAllSlots()
			}
			v.mu.RUnlock()

			byteData, err := json.Marshal(schema)
			if err != nil {
				log.Printf("failed to marshal registry cache, err: %v", err)
				continue
			}
			if err := os.WriteFile(dataPath, byteData, os.ModePerm); err != nil {
				log.Printf("failed to checkpoint registry cache, err: %v", err)
				continue
			}
			log.Printf("registry checkpointed at: %s", time.Now().UTC().Format(time.RFC3339))
		}
	}()

	return v
}

func (vr *VirtualNamespaceRegistry) Resolve(namespace string) *VirtualNamespace {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return vr.namespaces[namespace]
}

func (vr *VirtualNamespaceRegistry) Register(v *VirtualNamespace) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	vr.namespaces[v.Name] = v
}

type VirtualNamespace struct {
	Name   string
	Hasher *ConsistentHash
}

func NewVirtualNamespace(name string) *VirtualNamespace {
	return &VirtualNamespace{
		Name:   name,
		Hasher: NewConsistentHash(50),
	}
}

func (v *VirtualNamespace) Add(namespace *Namespace) {
	v.Hasher.AddSlot(namespace.name)
}

func (v *VirtualNamespace) Remove(namespace *Namespace) {
	v.Hasher.RemoveSlot(namespace.name)
}
