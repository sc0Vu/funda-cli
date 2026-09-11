package client

import (
	"compress/gzip"
	"context"

	"fmt"
	"io"
	"net/http"

	"strings"
	"time"

	"github.com/North-web-dev/impersonate-http"
	"github.com/klauspost/compress/zstd"
)

type Client struct {
	HTTP *http.Client
}

func NewClient(profile string, timeout time.Duration) (*Client, error) {
	if profile == "" {
		profile = "chrome"
	}

	httpClient, ok := impersonate.NewByName(
		strings.ToLower(profile),
		impersonate.WithTimeout(timeout),
	)
	if !ok {
		return nil, fmt.Errorf("unknown browser profile %q", profile)
	}

	return &Client{HTTP: httpClient}, nil
}

func (c *Client) Get(ctx context.Context, uri string, headers http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", resp.Status)
	}

	var resReader io.Reader
	contentEncoding := resp.Header.Get("Content-Encoding")
	if !resp.Uncompressed {
		switch contentEncoding {
		case "":
			resReader = resp.Body
			break
		case "gzip":
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("create gzip reader: %w", err)
			}
			defer gzReader.Close()
			resReader = gzReader
			break
		case "zstd":
			zstdReader, err := zstd.NewReader(resp.Body)
			if err != nil {
				return nil, err
			}
			defer zstdReader.Close()
			resReader = zstdReader
		default:
			return nil, fmt.Errorf("unknown content encoding: %s", contentEncoding)
		}
	}

	body, err := io.ReadAll(resReader)
	if err != nil {
		return nil, err
	}
	return body, nil
}
