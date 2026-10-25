// Package github implements the sequential HTTP client for GitHub issues.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/cache"
)

// Public API request constants used by ghreport.
const (
	UserAgent  = "ghreport/1.0 (+https://git.miem.hse.ru/blank5204543/vkworkspace)"
	APIVersion = "2026-03-10"
	maxBody    = 16 << 20
)

var nextLink = regexp.MustCompile(`<([^>]+)>\s*;[^,]*\brel\s*=\s*"?next"?`)

// ErrorCode classifies repository and fatal API failures.
type ErrorCode string

// Error codes used by report generation.
const (
	NotFound        ErrorCode = "not_found"
	RateLimited     ErrorCode = "rate_limited"
	RequestFailed   ErrorCode = "request_failed"
	InvalidResponse ErrorCode = "invalid_response"
	Unauthorized    ErrorCode = "unauthorized"
)

// Page is a fetched response body and its Link-derived next URL.
type Page struct {
	Body      []byte
	Next      string
	FromCache bool
}

// TraceRecord describes one HTTP attempt in the JSON Lines journal.
type TraceRecord struct {
	TS        string  `json:"ts"`
	Method    string  `json:"method"`
	URL       string  `json:"url"`
	Status    int     `json:"status"`
	Attempt   int     `json:"attempt"`
	FromCache bool    `json:"from_cache"`
	Error     *string `json:"error"`
}

// Client performs conditional requests, retries, and trace recording.
type Client struct {
	HTTP  *http.Client
	Cache cache.Store
	Token string
	Trace io.Writer
	Sleep func(context.Context, time.Duration) error
	Now   func() time.Time
}

// Fetch obtains one page, using a cached body when GitHub returns 304.
func (c *Client) Fetch(ctx context.Context, address string) (Page, ErrorCode) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	redirectClient := *httpClient
	previousCheckRedirect := redirectClient.CheckRedirect
	traceAttempt := 0
	requestURL := address
	record := func(address string, status int, fromCache bool, problem string) error {
		traceAttempt++
		return c.record(address, status, traceAttempt, fromCache, problem)
	}
	var redirectTraceErr error
	redirectClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if previousCheckRedirect != nil {
			if err := previousCheckRedirect(req, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if len(via) > 0 && req.Response != nil {
			redirectTraceErr = record(via[len(via)-1].URL.String(), req.Response.StatusCode, false, "")
			if redirectTraceErr != nil {
				return redirectTraceErr
			}
		}
		requestURL = req.URL.String()
		return nil
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	entry, hasCache := c.Cache.Load(address)
	for attempt := 1; attempt <= 4; attempt++ {
		requestURL = address
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, address, nil)
		if err != nil {
			cancel()
			return Page{}, RequestFailed
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("X-GitHub-Api-Version", APIVersion)
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		if hasCache {
			req.Header.Set("If-None-Match", entry.ETag)
		}
		resp, err := redirectClient.Do(req)
		if err != nil {
			cancel()
			if redirectTraceErr != nil {
				return Page{}, RequestFailed
			}
			if resp != nil {
				if record(resp.Request.URL.String(), resp.StatusCode, false, "redirect") != nil {
					return Page{}, RequestFailed
				}
				return Page{}, RequestFailed
			}
			kind := "network"
			if errors.Is(err, context.DeadlineExceeded) || requestCtx.Err() == context.DeadlineExceeded {
				kind = "timeout"
			}
			if record(requestURL, 0, false, kind) != nil {
				return Page{}, RequestFailed
			}
			if attempt == 4 || sleep(ctx, retryDelay(attempt)) != nil {
				return Page{}, RequestFailed
			}
			continue
		}
		responseURL := resp.Request.URL.String()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		closeErr := resp.Body.Close()
		cancel()
		if readErr != nil || closeErr != nil || len(body) > maxBody {
			if record(responseURL, resp.StatusCode, false, "read_error") != nil {
				return Page{}, RequestFailed
			}
			if attempt == 4 || sleep(ctx, retryDelay(attempt)) != nil {
				return Page{}, RequestFailed
			}
			continue
		}
		switch resp.StatusCode {
		case http.StatusOK:
			if !json.Valid(body) {
				_ = record(responseURL, resp.StatusCode, false, "invalid_json")
				return Page{}, InvalidResponse
			}
			link := resp.Header.Get("Link")
			next, err := parseNext(address, link)
			if err != nil {
				_ = record(responseURL, resp.StatusCode, false, "invalid_link")
				return Page{}, InvalidResponse
			}
			if err := c.Cache.Save(cache.Entry{URL: address, ETag: resp.Header.Get("ETag"), Link: link, Body: body}); err != nil {
				_ = record(responseURL, resp.StatusCode, false, "cache_write")
				return Page{}, RequestFailed
			}
			if record(responseURL, resp.StatusCode, false, "") != nil {
				return Page{}, RequestFailed
			}
			return Page{Body: body, Next: next}, ""
		case http.StatusNotModified:
			if !hasCache {
				_ = record(responseURL, resp.StatusCode, false, "cache_miss")
				return Page{}, InvalidResponse
			}
			next, err := parseNext(address, entry.Link)
			if err != nil {
				_ = record(responseURL, resp.StatusCode, false, "invalid_link")
				return Page{}, InvalidResponse
			}
			if record(responseURL, resp.StatusCode, true, "") != nil {
				return Page{}, RequestFailed
			}
			return Page{Body: entry.Body, Next: next, FromCache: true}, ""
		case http.StatusUnauthorized:
			_ = record(responseURL, resp.StatusCode, false, "")
			return Page{}, Unauthorized
		case http.StatusNotFound:
			_ = record(responseURL, resp.StatusCode, false, "")
			return Page{}, NotFound
		default:
			if rateLimitResponse(resp, body) {
				if record(responseURL, resp.StatusCode, false, "") != nil {
					return Page{}, RequestFailed
				}
				if attempt == 4 || sleep(ctx, rateDelay(resp.Header, c.now())) != nil {
					return Page{}, RateLimited
				}
				continue
			}
			if record(responseURL, resp.StatusCode, false, "") != nil {
				return Page{}, RequestFailed
			}
			if resp.StatusCode >= 500 && attempt < 4 && sleep(ctx, retryDelay(attempt)) == nil {
				continue
			}
			return Page{}, RequestFailed
		}
	}
	return Page{}, RequestFailed
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) record(address string, status, attempt int, fromCache bool, problem string) error {
	if c.Trace == nil {
		return nil
	}
	record := TraceRecord{TS: c.now().UTC().Format(time.RFC3339), Method: http.MethodGet, URL: address, Status: status, Attempt: attempt, FromCache: fromCache}
	if problem != "" {
		record.Error = &problem
	}
	return json.NewEncoder(c.Trace).Encode(record)
}

func retryDelay(attempt int) time.Duration { return time.Second << (attempt - 1) }

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func rateLimitResponse(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return false
	}
	message := strings.ToLower(string(body))
	return resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(message, "rate limit") || strings.Contains(message, "abuse detection")
}

func rateDelay(header http.Header, now time.Time) time.Duration {
	if retryAfter := header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.ParseFloat(retryAfter, 64); err == nil && seconds >= 0 {
			return time.Duration(seconds * float64(time.Second))
		}
		if instant, err := http.ParseTime(retryAfter); err == nil {
			return max(0, instant.Sub(now))
		}
	}
	if reset, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		return max(0, time.Unix(reset, 0).Sub(now))
	}
	return 60 * time.Second
}

func parseNext(current, link string) (string, error) {
	match := nextLink.FindStringSubmatch(link)
	if match == nil {
		return "", nil
	}
	base, err := url.Parse(current)
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(match[1])
	if err != nil {
		return "", err
	}
	next := base.ResolveReference(relative)
	if (next.Scheme != "http" && next.Scheme != "https") || next.Host == "" || next.User != nil || next.Fragment != "" {
		return "", fmt.Errorf("invalid next page URL")
	}
	return next.String(), nil
}
