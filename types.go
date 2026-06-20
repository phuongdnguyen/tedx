package main

import "strings"

type Namespace struct {
	cluster *Cluster
	name    string
}

type Cluster struct {
	name       string
	namespaces []*Namespace
	address    string
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
type VirtualNamespaceRegistry map[string]*VirtualNamespace

func NewVirtualNamespaceRegistry(virtualNamespaces []*VirtualNamespace) *VirtualNamespaceRegistry {
	v := make(VirtualNamespaceRegistry)
	for _, namespaceData := range virtualNamespaces {
		v[namespaceData.Name] = namespaceData
	}
	return &v
}

func (vr *VirtualNamespaceRegistry) Resolve(namespace string) *VirtualNamespace {
	return (*vr)[namespace]
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
