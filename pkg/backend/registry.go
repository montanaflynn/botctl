package backend

import (
	"fmt"
	"sort"
)

var registry = map[string]Backend{}

// Register adds a backend to the registry.
func Register(b Backend) {
	registry[b.Name()] = b
}

// Get returns a backend by name. Defaults to "claude" when name is empty.
func Get(name string) (Backend, error) {
	if name == "" {
		name = "claude"
	}
	b, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q (available: %v)", name, Names())
	}
	return b, nil
}

// Names returns all registered backend names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
