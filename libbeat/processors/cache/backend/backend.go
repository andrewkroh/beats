package backend

type Backend interface {
	Lookup(key string) (interface{}, error)

	Store(key string, value interface{}) error

	Delete(key string) error
}

type Config struct {
	ID   string      `config:"id"`
	Bolt *BoltConfig `config:"bolt"`
}
