package main

type StatsDB struct {
	// TODO: add namespace status
	namespaces map[string]int
}

func (db *StatsDB) Reset() {
	for k := range db.namespaces {
		db.namespaces[k] = 0
	}
}
func (db *StatsDB) Add(namespace string, value int) {
	db.namespaces[namespace] = value
}

func (db *StatsDB) Inc(namespace string) {
	db.namespaces[namespace]++
}

func (db *StatsDB) Remove(namespace string) {
	delete(db.namespaces, namespace)
}

func (db *StatsDB) Dump() map[string]int {
	return db.namespaces
}

func (db *StatsDB) Get(namespace string) int {
	return db.namespaces[namespace]
}
