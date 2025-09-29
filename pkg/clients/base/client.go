package base

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hsn0918/rag/pkg/config"
)

const (
	DefaultTimeout      = 30 * time.Second
	DefaultReadTimeout  = 60 * time.Second
	DefaultWriteTimeout = 30 * time.Second
)

type ClientError struct {
	Op         string
	Service    string
	StatusCode int
	Err        error
}

func (e *ClientError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("client: %s %s failed with status %d: %v", e.Service, e.Op, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("client: %s %s failed: %v", e.Service, e.Op, e.Err)
}

func (e *ClientError) Unwrap() error { return e.Err }

func NewClientError(service, op string, err error) *ClientError {
	return &ClientError{Op: op, Service: service, Err: err}
}

func NewHTTPError(service, op string, statusCode int, body string) *ClientError {
	return &ClientError{Op: op, Service: service, StatusCode: statusCode, Err: fmt.Errorf("HTTP %d: %s", statusCode, body)}
}

type HTTPClient struct {
	client  *resty.Client
	service string
}

func NewHTTPClient(service string, cfg config.ServiceConfig, timeout time.Duration) *HTTPClient {
	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetHeader("Authorization", "Bearer "+cfg.APIKey).
		SetHeader("Content-Type", "application/json").
		SetTimeout(timeout).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	client.AddRetryCondition(func(r *resty.Response, err error) bool { return err != nil || r.StatusCode() >= 500 })

	return &HTTPClient{client: client, service: service}
}

func (h *HTTPClient) Post(endpoint string, body interface{}, result interface{}) error {
	resp, err := h.client.R().SetBody(body).SetResult(result).Post(endpoint)
	if err != nil {
		return NewClientError(h.service, "POST "+endpoint, err)
	}
	if resp.StatusCode() != 200 {
		return NewHTTPError(h.service, "POST "+endpoint, resp.StatusCode(), resp.String())
	}
	return nil
}

func (h *HTTPClient) Get(endpoint string, params map[string]string, result interface{}) error {
	req := h.client.R().SetResult(result)
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get(endpoint)
	if err != nil {
		return NewClientError(h.service, "GET "+endpoint, err)
	}
	if resp.StatusCode() != 200 {
		return NewHTTPError(h.service, "GET "+endpoint, resp.StatusCode(), resp.String())
	}
	return nil
}

func (h *HTTPClient) Put(url string, body interface{}) error {
	resp, err := resty.New().R().SetBody(body).Put(url)
	if err != nil {
		return NewClientError(h.service, "PUT", err)
	}
	if resp.StatusCode() != 200 {
		return NewHTTPError(h.service, "PUT", resp.StatusCode(), resp.String())
	}
	return nil
}

func (h *HTTPClient) GetRaw(url string) ([]byte, error) {
	resp, err := resty.New().R().Get(url)
	if err != nil {
		return nil, NewClientError(h.service, "GET raw", err)
	}
	if resp.StatusCode() != 200 {
		return nil, NewHTTPError(h.service, "GET raw", resp.StatusCode(), resp.String())
	}
	return resp.Body(), nil
}

func IsRetryableError(err error) bool {
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		return false
	}
	return clientErr.StatusCode >= 500 || clientErr.StatusCode == 0
}
