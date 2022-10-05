package cache

import (
	"errors"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/mitchellh/hashstructure"
	"strings"
)

type Backend interface {
	Get(key string) (interface{}, error)

	Put(key string, value interface{}) error

	Delete(key string) error
}

type Config struct {
	ID   string      `config:"id" validate:"required"`
	Bolt *BoltConfig `config:"bolt"`
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("id must be non-empty")
	}

	if c.Bolt == nil {
		return errors.New("a cache backend type must be configured")
	}

	return nil
}

func New(config Config) (Backend, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	log := logp.L().Named("javascript.cache").With("id", config.ID)

	hashstructure.Hash(config, nil)
	// TODO: Lookup or created based on a hash of the config.
	switch {
	case config.Bolt != nil:
		return newBolt(log, *config.Bolt)
	default:
		return nil, errors.New("invalid cache config: missing a backend config")
	}
}
