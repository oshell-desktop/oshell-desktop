package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type UpdateHTTPError struct {
	Status  int
	RetryAt time.Time
}

func (e *UpdateHTTPError) Error() string {
	switch e.Status {
	case 404:
		return "更新站点尚未部署，或仓库地址不正确"
	case 403, 429:
		return "更新服务暂时限制访问，请稍后重试"
	}
	return fmt.Sprintf("更新服务返回 HTTP %d", e.Status)
}

func updateHTTPError(response *http.Response) *UpdateHTTPError {
	failure := &UpdateHTTPError{Status: response.StatusCode}
	retry := response.Header.Get("Retry-After")
	if seconds, err := strconv.ParseInt(retry, 10, 32); err == nil && seconds >= 0 {
		failure.RetryAt = time.Now().Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(retry); err == nil {
		failure.RetryAt = date
	}
	return failure
}

// Each configured source owns a separate cache. A failed refresh discards stale
// success data. Concurrent checks coalesce without an idle polling goroutine.
type UpdateMetadataCache struct {
	mu      sync.Mutex
	data    []byte
	expires time.Time
	failure error
	retry   time.Time
	clock   func() time.Time
}

func (c *UpdateMetadataCache) Value(ctx context.Context, load func(context.Context) ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	now := time.Now
	if c.clock != nil {
		now = c.clock
	}
	if c.data != nil && now().Before(c.expires) {
		return append([]byte(nil), c.data...), nil
	}
	if c.failure != nil && now().Before(c.retry) {
		return nil, c.failure
	}
	data, e := load(ctx)
	if e == nil {
		c.data = append([]byte(nil), data...)
		c.expires = now().Add(300 * time.Second)
		c.failure = nil
		return append([]byte(nil), data...), nil
	}
	c.data = nil
	// User cancellation is not a server failure and must not block a later check.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	delay := time.Minute
	if failure, ok := e.(*UpdateHTTPError); ok && !failure.RetryAt.IsZero() {
		delay = failure.RetryAt.Sub(now())
		delay = max(time.Minute, min(delay, 24*time.Hour))
	}
	c.failure = e
	c.retry = now().Add(delay)
	return nil, e
}
func FetchUpdateMetadata(ctx context.Context, source UpdateSource) ([]byte, error) {
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, source.MetadataURL(), nil)
	if e != nil {
		return nil, e
	}
	request.Header.Set("User-Agent", "OShell Desktop")
	request.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(request)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("无法连接更新站点")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, updateHTTPError(response)
	}
	if response.ContentLength > UpdateMetadataLimit {
		return nil, fmt.Errorf("更新信息超过大小限制")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, UpdateMetadataLimit+1))
	if e != nil {
		return nil, e
	}
	if len(data) > UpdateMetadataLimit {
		return nil, fmt.Errorf("更新信息超过大小限制")
	}
	return data, nil
}
