package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"go.etcd.io/bbolt"
)

var boltBucketName = []byte("cache-v1")

type Bolt struct {
	db     *bbolt.DB
	config BoltConfig
}

type BoltConfig struct {
	Path       string        `config:"path" validate:"required"`
	DefaultTTL time.Duration `config:"default_ttl"`
}

func (c *BoltConfig) InitDefaults() {
	c.DefaultTTL = time.Hour
}

var _ Backend = (*Bolt)(nil)

func newBolt(id string, cfg *BoltConfig) (*Bolt, error) {
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = time.Hour
	}

	dbPath := cfg.Path
	if filepath.Ext(cfg.Path) != ".db" {
		dbPath += ".db"
	}

	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 10 * time.Second})
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

	fmt.Printf("CACHE stats=%#v, config=%#v\n", db.Stats(), *cfg)

	return &Bolt{db: db, config: *cfg}, nil
}

func (c *Bolt) Lookup(key string) (interface{}, error) {
	var value *boltData

	err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(boltBucketName)

		data := b.Get([]byte(key))
		if data == nil {
			return nil
		}

		fmt.Println(string(data))
		if err := jsonDecode(data, &value); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if value == nil {
		fmt.Println("NOT_FOUND key=", key)
		return nil, nil
	}
	if value.Expired() {
		fmt.Println("EXPIRED key=", key)
		return nil, nil
	}

	fmt.Printf("FOUND key=%s data=%#v", key, value.Data)
	return value.Data, nil
}

func (c *Bolt) Store(key string, value interface{}) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(boltBucketName)

		data, err := jsonEncode(&boltData{
			ExpiresUnixEpoch: time.Now().Add(c.config.DefaultTTL).UnixNano(),
			Data:             value,
		})
		if err != nil {
			return err
		}

		fmt.Println(string(data))
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
	fmt.Println("Expiration time: ", time.Unix(0, d.ExpiresUnixEpoch))
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
