package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/logp"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/processors"
	cache "github.com/elastic/beats/v7/libbeat/processors/cache/backend"
	jsprocessor "github.com/elastic/beats/v7/libbeat/processors/script/javascript/module/processor"
)

func init() {
	processors.RegisterPlugin(processorName, New)
	jsprocessor.RegisterPlugin("Cache", New)
}

// Cache performs operations on a cache component. It can be used to `get`,
// `put`, or `delete` values in the cache.
type Cache struct {
	config  Config
	backend cache.Backend
	log     *logp.Logger
}

// New constructs a new Cache processor.
func New(c *config.C) (processors.Processor, error) {
	var config Config
	if err := c.Unpack(&config); err != nil {
		return nil, fmt.Errorf("fail to unpack the "+processorName+" processor configuration: %w", err)
	}

	return newCache(config)
}

// newCache returns a new Cache processor.
func newCache(config Config) (*Cache, error) {
	backend, err := cache.Registry.Lookup(config.CacheID)
	if err != nil {
		return nil, err
	}
	return &Cache{
		config:  config,
		backend: backend,
		log:     logp.NewLogger("processor." + processorName),
	}, nil
}

func (p *Cache) Run(evt *beat.Event) (*beat.Event, error) {
	switch p.config.Operation {
	case "get":
		return evt, p.get(evt)
	case "put":
		return evt, p.put(evt)
	case "delete":
		return evt, p.delete(evt)
	default:
		return evt, errors.New("invalid operation type")
	}
}

func (p *Cache) String() string {
	var buf strings.Builder
	buf.WriteString(processorName)
	buf.WriteByte('=')

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(p)
	return buf.String()
}

func (p *Cache) put(evt *beat.Event) error {
	key, err := getKey(p.config.KeyField, evt)
	if err != nil {
		return err
	}

	v, err := evt.GetValue(p.config.ValueField)
	if err != nil {
		return err
	}

	return p.backend.Store(key, v)
}

func (p *Cache) get(evt *beat.Event) error {
	key, err := getKey(p.config.KeyField, evt)
	if err != nil {
		return err
	}

	value, err := p.backend.Lookup(key)
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}

	_, err = evt.PutValue(p.config.TargetField, value)
	return err
}

func (p *Cache) delete(evt *beat.Event) error {
	key, err := getKey(p.config.KeyField, evt)
	if err != nil {
		return err
	}

	return p.backend.Delete(key)
}

func getKey(keyField string, evt *beat.Event) (string, error) {
	keyIfc, err := evt.GetValue(keyField)
	if err != nil {
		return "", err
	}

	var key string
	switch v := keyIfc.(type) {
	case string:
		key = v
	case uint8, uint16, uint32, uint64, uint,
		int8, int16, int32, int64, int:
		key = fmt.Sprintf("%v", v)
	default:
		return "", fmt.Errorf("invalid value type for key: found %T in %q", v, keyField)
	}

	return key, nil
}
