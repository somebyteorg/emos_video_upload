package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type EMOSClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewEMOSClient(cfg Config) (*EMOSClient, error) {
	if strings.TrimSpace(cfg.EMOSURL) == "" || strings.TrimSpace(cfg.EMOSToken) == "" {
		return nil, errors.New("EMOS_URL and EMOS_TOKEN are required")
	}
	return &EMOSClient{
		baseURL: strings.TrimRight(cfg.EMOSURL, "/"),
		token:   cfg.EMOSToken,
		client:  &http.Client{Timeout: cfg.HTTPTimeout},
	}, nil
}

func (c *EMOSClient) GetVideoTree(ctx context.Context, videoType, title, todbID string) ([]VideoTreeItem, error) {
	query := url.Values{}
	query.Set("type", videoType)
	if strings.TrimSpace(title) != "" {
		query.Set("title", title)
	}
	if strings.TrimSpace(todbID) != "" {
		query.Set("todb_id", todbID)
	}
	var result []VideoTreeItem
	if err := c.doJSON(ctx, http.MethodGet, "/api/video/tree?"+query.Encode(), nil, &result, http.StatusOK); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *EMOSClient) GetVideoBase(ctx context.Context, itemType string, itemID int64) (VideoBaseInfo, error) {
	query := url.Values{}
	query.Set("item_type", itemType)
	query.Set("item_id", fmt.Sprint(itemID))
	var result VideoBaseInfo
	if err := c.doJSON(ctx, http.MethodGet, "/api/upload/video/base?"+query.Encode(), nil, &result, http.StatusOK); err != nil {
		return VideoBaseInfo{}, err
	}
	return result, nil
}

func (c *EMOSClient) GetUploadToken(ctx context.Context, fileName, fileType string, fileSize int64, storage string) (UploadTokenResponse, error) {
	body := map[string]any{
		"type":         "video",
		"file_type":    fileType,
		"file_name":    fileName,
		"file_size":    fileSize,
		"file_storage": storage,
	}
	var result UploadTokenResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/upload/getUploadToken", body, &result, http.StatusOK); err != nil {
		return UploadTokenResponse{}, err
	}
	if strings.TrimSpace(result.Type) == "" || strings.TrimSpace(result.FileID) == "" {
		return UploadTokenResponse{}, errors.New("EMOS upload token response is incomplete")
	}
	return result, nil
}

func (c *EMOSClient) GetMultipartPresigns(ctx context.Context, fileID string, number int) ([]MultipartPresign, error) {
	body := map[string]int{"number": number}
	var result []MultipartPresign
	path := "/api/upload/multipart/" + url.PathEscape(fileID) + "/presign"
	if err := c.doJSON(ctx, http.MethodPost, path, body, &result, http.StatusOK); err != nil {
		return nil, err
	}
	if len(result) != number {
		return nil, fmt.Errorf("EMOS returned %d presigns, expected %d", len(result), number)
	}
	return result, nil
}

func (c *EMOSClient) CompleteMultipart(ctx context.Context, fileID string, parts []MultipartPart) error {
	path := "/api/upload/multipart/" + url.PathEscape(fileID) + "/complete"
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{"parts": parts}, nil, http.StatusNoContent, http.StatusOK)
}

func (c *EMOSClient) SaveVideo(ctx context.Context, itemType string, itemID int64, fileID string, metadata map[string]any) (SaveVideoResponse, error) {
	body := map[string]any{
		"item_type":     itemType,
		"item_id":       itemID,
		"file_id":       fileID,
		"file_metadata": metadata,
	}
	var result SaveVideoResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/upload/video/save", body, &result, http.StatusOK, http.StatusNoContent); err != nil {
		return SaveVideoResponse{}, err
	}
	return result, nil
}

func (c *EMOSClient) doJSON(ctx context.Context, method, path string, payload any, output any, expected ...int) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode EMOS request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("EMOS request %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read EMOS response: %w", err)
	}
	if !statusExpected(response.StatusCode, expected) {
		return &HTTPError{Method: method, Path: path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	if output == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode EMOS response %s %s: %w", method, path, err)
	}
	return nil
}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s returned HTTP %d", e.Method, e.Path, e.StatusCode)
	}
	return fmt.Sprintf("%s %s returned HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func statusExpected(actual int, expected []int) bool {
	for _, candidate := range expected {
		if actual == candidate {
			return true
		}
	}
	return false
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return time.Duration(1<<min(attempt-1, 4)) * 500 * time.Millisecond
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
