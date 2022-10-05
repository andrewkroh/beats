package cache

import (
	"time"
)

const (
	processorName = "cache"
)

// Config contains the configuration options for the cache processor.
type Config struct {
	// ID of the cache component to operate on.
	CacheID string `config:"cache_id" validate:"required"`

	// Ignore failures for the processor.
	IgnoreFailure bool `config:"ignore_failure"`

	// If true and field does not exist or is null, the processor quietly
	// returns without modifying the document.
	IgnoreMissing bool `config:"ignore_missing"`

	// Field that contains the key.
	KeyField string `config:"key_field" validate:"required"`

	// Operation to perform. One of `get`, `put`, `delete`.
	Operation string `config:"operation" validate:"required"`

	// The field to assign the output value to, by default field is updated
	// in-place.
	TargetField string `config:"target_field"`

	// Time-to-live for persisted key-value pairs. Not all cache component
	// types support per item TTLs.
	TTL time.Duration `config:"ttl"`

	// Field that contains the value to persist.
	ValueField string `config:"value_field"`
}

// InitDefaults initializes the configuration options to their default values.
func (c *Config) InitDefaults() {
	c.IgnoreFailure = false
	c.IgnoreMissing = false
	c.TTL = time.Hour
}
