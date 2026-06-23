package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"

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
	db         *redis.Client
}

// GetSize returns the number of virtual namespaces (for fallback logic)
func (vr *VirtualNamespaceRegistry) GetSize() int {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return len(vr.namespaces)
}

func NewVirtualNamespaceRegistry(redisAddr string) *VirtualNamespaceRegistry {
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	ctx := context.Background()
	if _, err := client.Ping(ctx).Result(); err != nil {
		log.Printf("Warning: failed to connect  at %s for registry: %v", redisAddr, err)
	} else {
		log.Printf("Connected to %s for registry", redisAddr)
	}

	v := &VirtualNamespaceRegistry{
		namespaces: make(map[string]*VirtualNamespace),
		db:         client,
	}

	// Read if exist
	var namespaceData []byte
	var err error

	redisData, redisErr := client.Get(ctx, "virtual_namespace_registry").Result()
	if redisErr == nil && redisData != "" {
		namespaceData = []byte(redisData)
		log.Printf("Loaded virtual namespace registry from redis")
	}

	if len(namespaceData) > 0 {
		var newSchema map[string]map[string]NamespaceStatus
		err = json.Unmarshal(namespaceData, &newSchema)
		if err == nil {
			for virtualName, slotsMap := range newSchema {
				vNs := NewVirtualNamespace(virtualName)
				for slot, status := range slotsMap {
					if status == NamespaceActive {
						vNs.Add(&Namespace{name: slot})
					} else {
						vNs.Namespaces[slot] = status
					}
				}
				v.namespaces[virtualName] = vNs
			}
		}
		log.Printf("Loaded %d virtual namespaces", len(v.namespaces))
	}

	// Checkpointing routine
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			v.mu.RLock()
			schema := make(map[string]map[string]NamespaceStatus)
			for name, vNs := range v.namespaces {
				schema[name] = vNs.GetAllNamespacesWithStatus()
			}
			v.mu.RUnlock()

			byteData, err := json.Marshal(schema)
			if err != nil {
				log.Printf("failed to marshal registry cache, err: %v", err)
				continue
			}

			// Checkpoint
			err = v.db.Set(context.Background(), "virtual_namespace_registry", string(byteData), 0).Err()
			if err != nil {
				log.Printf("failed to checkpoint registry, err: %v", err)
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

type NamespaceStatus string

const (
	NamespaceActive   NamespaceStatus = "active"
	NamespaceCordoned NamespaceStatus = "cordoned"
)

type VirtualNamespace struct {
	Name       string
	Ring       *ConsistentHash
	Namespaces map[string]NamespaceStatus
	mu         sync.RWMutex
}

func NewVirtualNamespace(name string) *VirtualNamespace {
	return &VirtualNamespace{
		Name:       name,
		Ring:       NewConsistentHash(50),
		Namespaces: make(map[string]NamespaceStatus),
	}
}

func (v *VirtualNamespace) Add(namespace *Namespace) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Namespaces[namespace.name] = NamespaceActive
	v.Ring.AddSlot(namespace.name)
}

func (v *VirtualNamespace) Remove(namespace *Namespace) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, exists := v.Namespaces[namespace.name]; exists {
		v.Namespaces[namespace.name] = NamespaceCordoned
	}
	v.Ring.RemoveSlot(namespace.name)
}

func (v *VirtualNamespace) GetAllNamespacesWithStatus() map[string]NamespaceStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	result := make(map[string]NamespaceStatus, len(v.Namespaces))
	for k, val := range v.Namespaces {
		result[k] = val
	}
	return result
}

func (v *VirtualNamespace) GetAllNamespaces() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var result []string
	for k := range v.Namespaces {
		result = append(result, k)
	}
	return result
}
