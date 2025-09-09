// Package doc2x provides a client for Doc2X document parsing service.
package doc2x

import (
	"fmt"
	"strings"
	"time"

	"github.com/hsn0918/rag/internal/clients/base"
	"github.com/hsn0918/rag/internal/config"
)

// Service name for error reporting
const serviceName = "doc2x"

// Default timeouts for Doc2X operations
const (
	DefaultTimeout    = 30 * time.Second
	ProcessingTimeout = 5 * time.Minute // for long-running parsing operations
)

// DocumentParser defines the interface for document parsing operations.
type DocumentParser interface {
	UploadPDF(pdfData []byte) (*UploadResponse, error)
	PreUpload() (*PreUploadResponse, error)
	UploadToPresignedURL(url string, fileData []byte) error
	GetStatus(uid string) (*StatusResponse, error)
	ConvertParse(req ConvertRequest) (*ConvertResponse, error)
	GetConvertResult(uid string) (*ConvertResultResponse, error)
	DownloadFile(url string) ([]byte, error)
	WaitForParsing(uid string, pollInterval time.Duration) (*StatusResponse, error)
	WaitForConversion(uid string, pollInterval time.Duration) (*ConvertResultResponse, error)
}

// Client provides Doc2X document parsing functionality.
// It wraps the HTTP client with domain-specific methods.
type Client struct {
	httpClient *base.HTTPClient
	cfg        config.ServiceConfig
}

// Compile-time check to ensure Client implements DocumentParser interface
var _ DocumentParser = (*Client)(nil)

// NewClient creates a new Doc2X client with standardized configuration.
// It uses the base HTTP client for consistent error handling and retry logic.
func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(serviceName, cfg, DefaultTimeout)

	return &Client{
		httpClient: httpClient,
		cfg:        cfg,
	}
}

type UploadResponse struct {
	Code string `json:"code"`
	Data struct {
		UID string `json:"uid"`
	} `json:"data"`
}

type PreUploadResponse struct {
	Code string `json:"code"`
	Data struct {
		UID string `json:"uid"`
		URL string `json:"url"`
	} `json:"data"`
}

type StatusResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg,omitempty"`
	Data *struct {
		Progress int    `json:"progress"`
		Status   string `json:"status"`
		Detail   string `json:"detail"`
		Result   *struct {
			Version string `json:"version"`
			Pages   []struct {
				URL        string `json:"url"`
				PageIdx    int    `json:"page_idx"`
				PageWidth  int    `json:"page_width"`
				PageHeight int    `json:"page_height"`
				Md         string `json:"md"`
			} `json:"pages"`
		} `json:"result"`
	} `json:"data"`
}

type ConvertRequest struct {
	UID                 string `json:"uid"`
	To                  string `json:"to"`
	FormulaMode         string `json:"formula_mode"`
	Filename            string `json:"filename,omitempty"`
	MergeCrossPageForms bool   `json:"merge_cross_page_forms,omitempty"`
}

type ConvertResponse struct {
	Code string `json:"code"`
	Data struct {
		Status string `json:"status"`
		URL    string `json:"url"`
	} `json:"data"`
}

type ConvertResultResponse struct {
	Code string `json:"code"`
	Data struct {
		Status string `json:"status"`
		URL    string `json:"url"`
	} `json:"data"`
}

// UploadPDF uploads PDF data for parsing.
// It returns the upload response containing the UID for tracking.
func (c *Client) UploadPDF(pdfData []byte) (*UploadResponse, error) {
	var result UploadResponse
	if err := c.httpClient.Post("/api/v2/parse/pdf", pdfData, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PreUpload initiates a presigned upload flow.
// It returns presigned URL for direct file upload.
func (c *Client) PreUpload() (*PreUploadResponse, error) {
	var result PreUploadResponse
	if err := c.httpClient.Post("/api/v2/parse/preupload", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UploadToPresignedURL uploads file data to a presigned URL.
// This is used in conjunction with PreUpload for large file uploads.
func (c *Client) UploadToPresignedURL(url string, fileData []byte) error {
	return c.httpClient.Put(url, fileData)
}

// GetStatus checks the parsing status for a given UID.
// It returns detailed status information including progress and results.
func (c *Client) GetStatus(uid string) (*StatusResponse, error) {
	var result StatusResponse
	params := map[string]string{"uid": uid}
	if err := c.httpClient.Get("/api/v2/parse/status", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ConvertParse initiates document conversion with specified parameters.
// It returns conversion tracking information.
func (c *Client) ConvertParse(req ConvertRequest) (*ConvertResponse, error) {
	var result ConvertResponse
	if err := c.httpClient.Post("/api/v2/convert/parse", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetConvertResult retrieves conversion results for a given UID.
// It returns the final converted document information.
func (c *Client) GetConvertResult(uid string) (*ConvertResultResponse, error) {
	var result ConvertResultResponse
	params := map[string]string{"uid": uid}
	if err := c.httpClient.Get("/api/v2/convert/parse/result", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DownloadFile downloads a file from the given URL.
// It handles URL unescaping and returns the raw file content.
func (c *Client) DownloadFile(url string) ([]byte, error) {
	// Fix URL encoding issues
	url = strings.ReplaceAll(url, "\\u0026", "&")
	return c.httpClient.GetRaw(url)
}

// WaitForParsing polls the parsing status until completion or failure.
// It uses the provided poll interval to check status periodically.
// Returns the final status or an error if parsing fails.
func (c *Client) WaitForParsing(uid string, pollInterval time.Duration) (*StatusResponse, error) {
	for {
		status, err := c.GetStatus(uid)
		if err != nil {
			return nil, err
		}

		if status.Code != "success" {
			return nil, base.NewClientError(serviceName, "wait for parsing",
				fmt.Errorf("parse failed: %s - %s", status.Code, status.Msg))
		}

		switch status.Data.Status {
		case "success":
			return status, nil
		case "failed":
			return nil, base.NewClientError(serviceName, "wait for parsing",
				fmt.Errorf("parse failed: %s", status.Data.Detail))
		case "processing":
			time.Sleep(pollInterval)
		default:
			// Unknown status, continue polling
			time.Sleep(pollInterval)
		}
	}
}

// WaitForConversion polls the conversion status until completion or failure.
// It uses the provided poll interval to check status periodically.
// Returns the final result or an error if conversion fails.
func (c *Client) WaitForConversion(uid string, pollInterval time.Duration) (*ConvertResultResponse, error) {
	for {
		result, err := c.GetConvertResult(uid)
		if err != nil {
			return nil, err
		}

		if result.Code != "success" {
			return nil, base.NewClientError(serviceName, "wait for conversion",
				fmt.Errorf("convert failed: %s", result.Code))
		}

		switch result.Data.Status {
		case "success":
			return result, nil
		case "failed":
			return nil, base.NewClientError(serviceName, "wait for conversion",
				fmt.Errorf("convert failed"))
		case "processing":
			time.Sleep(pollInterval)
		default:
			// Unknown status, continue polling
			time.Sleep(pollInterval)
		}
	}
}
