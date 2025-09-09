package base

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hsn0918/rag/internal/config"
)

// Default timeout values for HTTP clients
const (
	DefaultTimeout      = 30 * time.Second
	DefaultReadTimeout  = 60 * time.Second
	DefaultWriteTimeout = 30 * time.Second
)

// ClientError represents HTTP client operation errors with context.
type ClientError struct {
	Op         string // the operation that failed
	Service    string // the service name
	StatusCode int    // HTTP status code (if applicable)
	Err        error  // the underlying error
}

func (e *ClientError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("client: %s %s failed with status %d: %v",
			e.Service, e.Op, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("client: %s %s failed: %v", e.Service, e.Op, e.Err)
}

func (e *ClientError) Unwrap() error {
	return e.Err
}

// NewClientError creates a new ClientError with the given parameters.
func NewClientError(service, op string, err error) *ClientError {
	return &ClientError{
		Op:      op,
		Service: service,
		Err:     err,
	}
}

// NewHTTPError creates a new ClientError for HTTP status code errors.
func NewHTTPError(service, op string, statusCode int, body string) *ClientError {
	return &ClientError{
		Op:         op,
		Service:    service,
		StatusCode: statusCode,
		Err:        fmt.Errorf("HTTP %d: %s", statusCode, body),
	}
}

// HTTPClient provides a standardized HTTP client configuration.
// It encapsulates common patterns used across all service clients.
type HTTPClient struct {
	client  *resty.Client
	service string // service name for error reporting
}

// NewHTTPClient creates a new HTTP client with standard configuration.
// It applies consistent timeout, headers, and middleware settings.
func NewHTTPClient(service string, cfg config.ServiceConfig, timeout time.Duration) *HTTPClient {
	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetHeader("Authorization", "Bearer "+cfg.APIKey).
		SetHeader("Content-Type", "application/json").
		SetTimeout(timeout).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	// Add retry conditions for transient failures
	client.AddRetryCondition(func(r *resty.Response, err error) bool {
		return err != nil || r.StatusCode() >= 500
	})

	return &HTTPClient{
		client:  client,
		service: service,
	}
}

// Post performs a POST request with standardized error handling.
func (h *HTTPClient) Post(endpoint string, body interface{}, result interface{}) error {
	resp, err := h.client.R().
		SetBody(body).
		SetResult(result).
		Post(endpoint)

	if err != nil {
		return NewClientError(h.service, "POST "+endpoint, err)
	}

	if resp.StatusCode() != 200 {
		return NewHTTPError(h.service, "POST "+endpoint, resp.StatusCode(), resp.String())
	}

	return nil
}

// Get performs a GET request with standardized error handling.
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

// Put performs a PUT request with standardized error handling.
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

// GetRaw performs a GET request and returns raw response body.
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

// IsRetryableError reports whether an error is retryable.
// This helps upper layers decide whether to retry operations.
func IsRetryableError(err error) bool {
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		return false
	}

	// Consider 5xx status codes and network errors as retryable
	return clientErr.StatusCode >= 500 || clientErr.StatusCode == 0
}
