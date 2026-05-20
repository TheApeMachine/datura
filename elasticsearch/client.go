package elasticsearch

import (
	"fmt"
	"strings"

	esv8 "github.com/elastic/go-elasticsearch/v9"
)

/*
Config holds connection details for an Elasticsearch cluster accessed via the official Go client.

Provide at least one entry in Addresses. Use APIKey for Elastic Cloud / API key auth, or Username
and Password for basic auth. CACert is optional PEM bytes for custom certificate authorities when
using TLS.
*/
type Config struct {
	Addresses []string
	Username  string
	Password  string
	APIKey    string
	CACert    []byte
}

/*
Client wraps go-elasticsearch with mosaic defaults. The native client is available for advanced
esapi calls. The same underlying client is safe for concurrent use per Elasticsearch client rules.
*/
type Client struct {
	es *esv8.Client
}

/*
NewClient builds a Client from cfg. At least one non-empty address is required after trimming.
*/
func NewClient(cfg Config) (*Client, error) {
	addrs := normalizeAddresses(cfg.Addresses)

	if len(addrs) == 0 {
		return nil, fmt.Errorf("elasticsearch: at least one address is required")
	}

	escfg := esv8.Config{
		Addresses: addrs,
	}

	apiKey := strings.TrimSpace(cfg.APIKey)

	if apiKey != "" {
		escfg.APIKey = apiKey
	} else if strings.TrimSpace(cfg.Username) != "" {
		escfg.Username = strings.TrimSpace(cfg.Username)
		escfg.Password = cfg.Password
	}

	if len(cfg.CACert) > 0 {
		escfg.CACert = cfg.CACert
	}

	es, err := esv8.NewClient(escfg)

	if err != nil {
		return nil, fmt.Errorf("elasticsearch: new client: %w", err)
	}

	return &Client{es: es}, nil
}

func splitCommaAddresses(raw string) []string {
	var addrs []string

	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)

		if part != "" {
			addrs = append(addrs, part)
		}
	}

	return addrs
}

/*
Native returns the underlying go-elasticsearch client for Index, Cluster, and other esapi entry
points not wrapped by Store.
*/
func (client *Client) Native() *esv8.Client {
	return client.es
}

func normalizeAddresses(addrs []string) []string {
	var out []string

	for _, a := range addrs {
		a = strings.TrimSpace(a)

		if a == "" {
			continue
		}

		out = append(out, a)
	}

	return out
}
