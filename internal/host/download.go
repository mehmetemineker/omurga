package host

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxRepositoryFileSize = 5 << 20

type Downloader interface {
	Download(ctx context.Context, url string) ([]byte, error)
}

type HTTPDownloader struct {
	Client *http.Client
}

func (d HTTPDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "omurga")

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned HTTP %s", response.Status)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maxRepositoryFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("download returned an empty file")
	}
	if len(data) > maxRepositoryFileSize {
		return nil, fmt.Errorf("download exceeded the %d byte limit", maxRepositoryFileSize)
	}
	return data, nil
}
