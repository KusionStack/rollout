package utils

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/golang/groupcache/lru"
)

const (
	defaultCacheSize = 200
	defaultTimeout   = time.Second * 30
)

type ClientConfig struct {
	CABundle []byte
}

type ClientManager struct {
	sync.RWMutex
	cache *lru.Cache
}

var ClientManagerSingleton = NewClientManager()

// NewClientManager construct ClientManager
func NewClientManager() *ClientManager {
	return &ClientManager{cache: lru.New(defaultCacheSize)}
}

// Client get HttpClient from cache if present or new instance if not present
func (cm *ClientManager) Client(cc *ClientConfig) (c *http.Client, err error) {
	cacheKey, err := json.Marshal(cc)
	if err != nil {
		return nil, err
	}

	client := cm.getClient(string(cacheKey))
	if client == nil {
		client, err = cm.newClient(cc)
		if err != nil {
			return nil, err
		} else {
			cm.addClient(string(cacheKey), client)
		}
	}
	return client, nil
}

func (cm *ClientManager) getClient(cacheKey string) *http.Client {
	cm.RLock()
	defer cm.RUnlock()
	if client, ok := cm.cache.Get(cacheKey); ok {
		return client.(*http.Client)
	}
	return nil
}

func (cm *ClientManager) addClient(cacheKey string, client *http.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.cache.Add(cacheKey, client)
}

func (cm *ClientManager) newClient(cc *ClientConfig) (*http.Client, error) {
	if cc.CABundle == nil {
		return &http.Client{Timeout: defaultTimeout}, nil
	}

	if bt, err := base64.StdEncoding.DecodeString(string(cc.CABundle)); err != nil {
		return nil, err
	} else {
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(bt)
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{ClientCAs: certPool},
		}
		return &http.Client{Transport: transport, Timeout: defaultTimeout}, nil
	}
}
