package doc2x

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hsn0918/rag/internal/config"
)


type Client struct {
	client *resty.Client
	config config.ServiceConfig
}

func NewClient(cfg config.ServiceConfig) *Client {
	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetHeader("Authorization", "Bearer "+cfg.APIKey).
		SetTimeout(30 * time.Second)

	return &Client{
		client: client,
		config: cfg,
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

func (c *Client) UploadPDF(pdfData []byte) (*UploadResponse, error) {
	var result UploadResponse
	resp, err := c.client.R().
		SetBody(pdfData).
		SetResult(&result).
		Post("/api/v2/parse/pdf")

	if err != nil {
		return nil, fmt.Errorf("upload pdf failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("upload pdf failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) PreUpload() (*PreUploadResponse, error) {
	var result PreUploadResponse
	resp, err := c.client.R().
		SetResult(&result).
		Post("/api/v2/parse/preupload")

	if err != nil {
		return nil, fmt.Errorf("preupload failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("preupload failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) UploadToPresignedURL(url string, fileData []byte) error {
	resp, err := resty.New().R().
		SetBody(fileData).
		Put(url)

	if err != nil {
		return fmt.Errorf("upload to presigned url failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("upload to presigned url failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

func (c *Client) GetStatus(uid string) (*StatusResponse, error) {
	var result StatusResponse
	resp, err := c.client.R().
		SetQueryParam("uid", uid).
		SetResult(&result).
		Get("/api/v2/parse/status")

	if err != nil {
		return nil, fmt.Errorf("get status failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("get status failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) ConvertParse(req ConvertRequest) (*ConvertResponse, error) {
	var result ConvertResponse
	resp, err := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&result).
		Post("/api/v2/convert/parse")

	if err != nil {
		return nil, fmt.Errorf("convert parse failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("convert parse failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) GetConvertResult(uid string) (*ConvertResultResponse, error) {
	var result ConvertResultResponse
	resp, err := c.client.R().
		SetQueryParam("uid", uid).
		SetResult(&result).
		Get("/api/v2/convert/parse/result")

	if err != nil {
		return nil, fmt.Errorf("get convert result failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("get convert result failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) DownloadFile(url string) ([]byte, error) {
	url = strings.ReplaceAll(url, "\\u0026", "&")

	resp, err := resty.New().R().Get(url)
	if err != nil {
		return nil, fmt.Errorf("download file failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("download file failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

func (c *Client) WaitForParsing(uid string, pollInterval time.Duration) (*StatusResponse, error) {
	for {
		status, err := c.GetStatus(uid)
		if err != nil {
			return nil, err
		}

		if status.Code != "success" {
			return nil, fmt.Errorf("parse failed: %s - %s", status.Code, status.Msg)
		}

		switch status.Data.Status {
		case "success":
			return status, nil
		case "failed":
			return nil, fmt.Errorf("parse failed: %s", status.Data.Detail)
		case "processing":
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
			return nil, fmt.Errorf("convert failed: %s", result.Code)
		}

		switch result.Data.Status {
		case "success":
			return result, nil
		case "failed":
			return nil, fmt.Errorf("convert failed")
		case "processing":
			time.Sleep(pollInterval)
		}
	}
}
