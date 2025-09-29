package rerank

import (
	"time"

	"github.com/hsn0918/rag/pkg/clients/base"
	"github.com/hsn0918/rag/pkg/config"
)

const (
	DefaultTimeout = 30 * time.Second
	ServiceName    = "rerank"
)

type Client struct {
	httpClient *base.HTTPClient
	config     config.ServiceConfig
}

func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(ServiceName, cfg, DefaultTimeout)
	return &Client{httpClient: httpClient, config: cfg}
}
