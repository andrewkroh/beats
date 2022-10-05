package cache

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent-libs/paths"
	"go.etcd.io/bbolt"
)

var boltBucketName = []byte("cache-v1")

type BoltConfig struct {
	Path       string        `config:"path" validate:"required"`
	DefaultTTL time.Duration `config:"default_ttl"`
}

func (c *BoltConfig) InitDefaults() {
	c.DefaultTTL = time.Hour
}

type Bolt struct {
	db     *bbolt.DB
	log    *logp.Logger
	config BoltConfig
}

var _ Backend = (*Bolt)(nil)

func newBolt(log *logp.Logger, cfg BoltConfig) (*Bolt, error) {
	log = log.Named("bolt")

	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = time.Hour
	}

	dbPath := cfg.Path
	if filepath.Ext(cfg.Path) != ".db" {
		dbPath += ".db"
	}
	if !filepath.IsAbs(dbPath) {
		// Resolve DB path relative to --path.data.
		dbPath = paths.Resolve(paths.Data, dbPath)
	}

	log.Debugf("Loading database from %v.", dbPath)
	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(boltBucketName)
		return err
	})
	if err != nil {
		return nil, err
	}

	return &Bolt{db: db, log: log, config: cfg}, nil
}

func (c *Bolt) Get(key string) (interface{}, error) {
	var value *boltData

	err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(boltBucketName)

		data := b.Get([]byte(key))
		if data == nil {
			return nil
		}

		if err := jsonDecode(data, &value); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if value == nil {
		c.log.Debugw("Key not found.", "key", key)
		return nil, nil
	}
	if value.Expired() {
		c.log.Debugw("Key expired.", "key", key, "expiration", value.ExpiresUnixEpoch)
		return nil, nil
	}

	c.log.Debugw("Found key.", "key", key)
	return value.Data, nil
}

func (c *Bolt) Put(key string, value interface{}) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(boltBucketName)

		data, err := jsonEncode(&boltData{
			ExpiresUnixEpoch: time.Now().Add(c.config.DefaultTTL).UnixNano(),
			Data:             value,
		})
		if err != nil {
			return err
		}

		return b.Put([]byte(key), data)
	})
}

func (c *Bolt) Delete(key string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(boltBucketName)
		return b.Delete([]byte(key))
	})
}

func (c *Bolt) close() error {
	return c.db.Close()
}

type boltData struct {
	ExpiresUnixEpoch int64       `json:"exp"`
	Data             interface{} `json:"data"`
}

func (d *boltData) Expired() bool {
	return time.Now().After(time.Unix(0, d.ExpiresUnixEpoch))
}

func jsonEncode(value *boltData) ([]byte, error) {
	buf := new(bytes.Buffer)

	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func jsonDecode(data []byte, out interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	// TODO: In order to preserve number formats we should add UseNumber
	// and then transform json.Number back to float/int via jsontransform.
	//dec.UseNumber()

	return dec.Decode(out)
}
