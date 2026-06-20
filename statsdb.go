package main

import "sync"

type StatsDB struct {
	// TODO: add namespace status
	namespaces map[string]int
	lock       sync.RWMutex
}

func (db *StatsDB) Reset() {
	db.lock.Lock()
	for k := range db.namespaces {
		db.namespaces[k] = 0
	}
	db.lock.Unlock()
}

func (db *StatsDB) Inc(namespace string) {
	db.lock.Lock()
	db.namespaces[namespace]++
	db.lock.Unlock()
}

func (db *StatsDB) Remove(namespace string) {
	db.lock.Lock()
	delete(db.namespaces, namespace)
	db.lock.Unlock()
}

func (db *StatsDB) Dump() map[string]int {
	db.lock.RLock()
	defer db.lock.RUnlock()
	return db.namespaces
}

func (db *StatsDB) Get(namespace string) int {
	db.lock.RLock()
	defer db.lock.RUnlock()
	return db.namespaces[namespace]
}
