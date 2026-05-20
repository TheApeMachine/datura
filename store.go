package datura

/*
Store is an interface that all data sources must implement.
*/
type Store interface {
	Get(key string) (string, error)
	Put(key string, value string) error
	Delete(key string) error
	Search(query string) ([]string, error)
}
