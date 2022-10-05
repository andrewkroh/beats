package backend

import (
	"fmt"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/prometheus/client_golang/prometheus"
	"io"
	"sync"
)

var Registry = &ComponentRegistry{}

type ComponentConfig struct {
	ID   string      `config:"id"`
	Bolt *BoltConfig `config:"bolt"`
}

type ComponentRegistry struct {
	cacheComponents map[string]Backend
	mutex           sync.Mutex
}

func (c *ComponentRegistry) ConfigureComponents(cfg []ComponentConfig) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, componentConfig := range cfg {
		_, found := c.cacheComponents[componentConfig.ID]
		if found {
			return fmt.Errorf("cache_component with id=%q is already registered", componentConfig.ID)
		}

		logp.NewLogger("processors.cache").Infof("Creating cache_component %v", componentConfig.ID)
		if err := c.configureComponent(componentConfig); err != nil {
			return err
		}
	}

	return nil
}

func (c *ComponentRegistry) configureComponent(cfg ComponentConfig) error {
	if c.cacheComponents == nil {
		c.cacheComponents = map[string]Backend{}
	}

	switch {
	case cfg.Bolt != nil:
		boltBackend, err := newBolt(cfg.ID, cfg.Bolt)
		if err != nil {
			return err
		}
		c.cacheComponents[cfg.ID] = boltBackend
	default:
		return fmt.Errorf("a cache backend must be configured for component id %q", cfg.ID)
	}

	return nil
}

func (c *ComponentRegistry) Lookup(id string) (Backend, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	backend, found := c.cacheComponents[id]
	if !found {
		return nil, fmt.Errorf("cache component id %q not found", id)
	}

	return backend, nil
}

func (c *ComponentRegistry) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var errs []error
	for _, b := range c.cacheComponents {
		if closer, ok := b.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	c.cacheComponents = nil

	// TODO: Determine a single multierr library to use an apply it everywhere.
	return prometheus.MultiError(errs)
}
