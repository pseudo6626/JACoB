package webfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"jacob/internal/buildinfo"
)

const (
	maxRequestBytes  = 1 << 20
	maxResponseBytes = 8 << 20
)

var allowedRequestHeaders = map[string]bool{
	"accept": true, "content-type": true, "authorization": true, "x-api-key": true,
	"if-none-match": true, "if-modified-since": true,
}

type Request struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type Response struct {
	URL         string            `json:"url"`
	Status      int               `json:"status"`
	StatusText  string            `json:"statusText"`
	Headers     map[string]string `json:"headers"`
	ContentType string            `json:"contentType,omitempty"`
	Body        string            `json:"body"`
	JSON        any               `json:"json,omitempty"`
	Bytes       int               `json:"bytes"`
}

type Client struct {
	http *http.Client
}

func New() *Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("host did not resolve")
			}
			for _, item := range ips {
				if unsafeIP(item.IP) {
					return nil, fmt.Errorf("target resolves to a local or private network address")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
	c := &Client{}
	c.http = &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return validateURL(req.URL)
		},
	}
	return c
}

func (c *Client) Fetch(ctx context.Context, in Request) (Response, error) {
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil {
		return Response{}, fmt.Errorf("invalid URL: %w", err)
	}
	if err := validateURL(u); err != nil {
		return Response{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return Response{}, errors.New("only GET and POST are available through net.fetch")
	}
	if len([]byte(in.Body)) > maxRequestBytes {
		return Response{}, fmt.Errorf("request body exceeds %d byte limit", maxRequestBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewBufferString(in.Body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("User-Agent", "JACoB/"+buildinfo.Version+" (+https://github.com/"+buildinfo.Repository+")")
	for k, v := range in.Headers {
		canon := http.CanonicalHeaderKey(strings.TrimSpace(k))
		if !allowedRequestHeaders[strings.ToLower(canon)] {
			continue
		}
		if len(v) > 8192 {
			return Response{}, fmt.Errorf("header %s is too large", canon)
		}
		req.Header.Set(canon, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	reader := io.LimitReader(resp.Body, maxResponseBytes+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return Response{}, err
	}
	if len(body) > maxResponseBytes {
		return Response{}, fmt.Errorf("response exceeds %d byte limit", maxResponseBytes)
	}

	headers := map[string]string{}
	for _, name := range []string{"Content-Type", "ETag", "Last-Modified", "Cache-Control"} {
		if v := resp.Header.Get(name); v != "" {
			headers[name] = v
		}
	}
	out := Response{
		URL: resp.Request.URL.String(), Status: resp.StatusCode, StatusText: resp.Status,
		Headers: headers, ContentType: resp.Header.Get("Content-Type"), Body: string(body), Bytes: len(body),
	}
	if strings.Contains(strings.ToLower(out.ContentType), "json") || json.Valid(body) {
		var decoded any
		if json.Unmarshal(body, &decoded) == nil {
			out.JSON = decoded
		}
	}
	return out, nil
}

func validateURL(u *url.URL) error {
	if u == nil {
		return errors.New("URL is required")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("net.fetch allows only http:// and https:// URLs")
	}
	if u.User != nil {
		return errors.New("credentials may not be embedded in the URL")
	}
	host := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."))
	if host == "" {
		return errors.New("URL host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return errors.New("local hostnames are blocked")
	}
	port := u.Port()
	if port != "" {
		want := "443"
		if u.Scheme == "http" {
			want = "80"
		}
		if port != want {
			return fmt.Errorf("non-standard port %s is blocked", strconv.Quote(port))
		}
	}
	if ip := net.ParseIP(host); ip != nil && unsafeIP(ip) {
		return errors.New("local or private network addresses are blocked")
	}
	return nil
}

func unsafeIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// Go's IsPrivate intentionally does not include carrier-grade NAT or the
	// protocol benchmark range. Neither is a public-Internet destination and
	// both can expose infrastructure on unusual local/provider networks.
	for _, raw := range []string{"100.64.0.0/10", "198.18.0.0/15"} {
		_, block, err := net.ParseCIDR(raw)
		if err == nil && block.Contains(ip) {
			return true
		}
	}
	return false
}
