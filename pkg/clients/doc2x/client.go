package doc2x

import (
	"fmt"
	"strings"
	"time"

	"github.com/hsn0918/rag/pkg/clients/base"
	"github.com/hsn0918/rag/pkg/config"
)

const serviceName = "doc2x"

const (
	DefaultTimeout    = 30 * time.Second
	ProcessingTimeout = 5 * time.Minute
)

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

type Client struct {
	httpClient *base.HTTPClient
	cfg        config.ServiceConfig
}

var _ DocumentParser = (*Client)(nil)

func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(serviceName, cfg, DefaultTimeout)
	return &Client{httpClient: httpClient, cfg: cfg}
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

func (c *Client) UploadPDF(pdfData []byte) (*UploadResponse, error) {
	var result UploadResponse
	if err := c.httpClient.Post("/api/v2/parse/pdf", pdfData, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) PreUpload() (*PreUploadResponse, error) {
	var result PreUploadResponse
	if err := c.httpClient.Post("/api/v2/parse/preupload", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) UploadToPresignedURL(url string, fileData []byte) error {
	return c.httpClient.Put(url, fileData)
}
func (c *Client) GetStatus(uid string) (*StatusResponse, error) {
	var result StatusResponse
	params := map[string]string{"uid": uid}
	if err := c.httpClient.Get("/api/v2/parse/status", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) ConvertParse(req ConvertRequest) (*ConvertResponse, error) {
	var result ConvertResponse
	if err := c.httpClient.Post("/api/v2/convert/parse", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) GetConvertResult(uid string) (*ConvertResultResponse, error) {
	var result ConvertResultResponse
	params := map[string]string{"uid": uid}
	if err := c.httpClient.Get("/api/v2/convert/parse/result", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) DownloadFile(url string) ([]byte, error) {
	url = strings.ReplaceAll(url, "\\u0026", "&")
	return c.httpClient.GetRaw(url)
}

func (c *Client) WaitForParsing(uid string, pollInterval time.Duration) (*StatusResponse, error) {
	for {
		status, err := c.GetStatus(uid)
		if err != nil {
			return nil, err
		}
		if status.Code != "success" {
			return nil, base.NewClientError(serviceName, "wait for parsing", fmt.Errorf("parse failed: %s - %s", status.Code, status.Msg))
		}
		switch status.Data.Status {
		case "success":
			return status, nil
		case "failed":
			return nil, base.NewClientError(serviceName, "wait for parsing", fmt.Errorf("parse failed: %s", status.Data.Detail))
		case "processing":
			time.Sleep(pollInterval)
		default:
			time.Sleep(pollInterval)
		}
	}
}

func (c *Client) WaitForConversion(uid string, pollInterval time.Duration) (*ConvertResultResponse, error) {
	for {
		result, err := c.GetConvertResult(uid)
		if err != nil {
			return nil, err
		}
		if result.Code != "success" {
			return nil, base.NewClientError(serviceName, "wait for conversion", fmt.Errorf("convert failed: %s", result.Code))
		}
		switch result.Data.Status {
		case "success":
			return result, nil
		case "failed":
			return nil, base.NewClientError(serviceName, "wait for conversion", fmt.Errorf("convert failed"))
		case "processing":
			time.Sleep(pollInterval)
		default:
			time.Sleep(pollInterval)
		}
	}
}
