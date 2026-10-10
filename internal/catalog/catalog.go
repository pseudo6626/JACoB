package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"jacob/internal/webfetch"
)

const spanshSystemURL = "https://spansh.co.uk/api/system/"

type Client struct {
	net *webfetch.Client
}

type SystemRequest struct {
	ID64     string `json:"id64"`
	Provider string `json:"provider,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type SystemResult struct {
	Found       bool           `json:"found"`
	Provider    string         `json:"provider"`
	Detail      string         `json:"detail"`
	ID64        string         `json:"id64"`
	Cached      bool           `json:"cached"`
	Revalidated bool           `json:"revalidated"`
	Summary     map[string]any `json:"summary,omitempty"`
	Record      map[string]any `json:"record,omitempty"`
}

func New(netClient *webfetch.Client) *Client { return &Client{net: netClient} }

func (c *Client) SystemGet(ctx context.Context, in SystemRequest) (SystemResult, error) {
	if c == nil || c.net == nil {
		return SystemResult{}, errors.New("system catalog is unavailable")
	}
	id := strings.TrimSpace(in.ID64)
	if id == "" {
		return SystemResult{}, errors.New("id64 is required")
	}
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return SystemResult{}, errors.New("id64 must be an unsigned decimal integer string")
	}
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider == "" {
		provider = "spansh"
	}
	if provider != "spansh" {
		return SystemResult{}, fmt.Errorf("unsupported catalog provider %q", provider)
	}
	detail := strings.ToLower(strings.TrimSpace(in.Detail))
	if detail == "" {
		detail = "summary"
	}
	if detail != "summary" && detail != "full" {
		return SystemResult{}, errors.New("detail must be summary or full")
	}

	interval, retry, ttl := 250, 2, 6*60*60*1000
	resp, err := c.net.FetchQueued(ctx, webfetch.QueueRequest{
		URL:           spanshSystemURL + id,
		Method:        "GET",
		Headers:       map[string]string{"Accept": "application/json"},
		Group:         "catalog:spansh",
		MinIntervalMs: &interval,
		Retry:         &retry,
		CacheTTLms:    &ttl,
	})
	if err != nil {
		return SystemResult{}, err
	}
	out := SystemResult{
		Found: true, Provider: provider, Detail: detail, ID64: id,
		Cached: resp.Cached, Revalidated: resp.Revalidated,
	}
	if resp.Status == 404 {
		out.Found = false
		return out, nil
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return SystemResult{}, fmt.Errorf("spansh system lookup returned HTTP %d", resp.Status)
	}

	record, err := decodeSpanshRecord(resp.Body)
	if err != nil {
		return SystemResult{}, err
	}
	out.Summary = summaryFromRecord(record, id)
	if detail == "full" {
		out.Record = record
	}
	return out, nil
}

func decodeSpanshRecord(body string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode Spansh system response: %w", err)
	}
	if raw, ok := root["record"]; ok {
		rec, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("Spansh system response record is not an object")
		}
		return rec, nil
	}
	return root, nil
}

func summaryFromRecord(record map[string]any, requestedID string) map[string]any {
	out := map[string]any{"id64": requestedID}
	if v := stringValue(record["id64"]); v != "" {
		out["id64"] = v
	}
	if v := stringValue(first(record, "name", "system_name", "systemName")); v != "" {
		out["name"] = v
	}
	if coords := mapValue(first(record, "coords", "coordinates")); coords != nil {
		out["coords"] = map[string]any{
			"x": numberValue(first(coords, "x")),
			"y": numberValue(first(coords, "y")),
			"z": numberValue(first(coords, "z")),
		}
	} else if x, y, z := numberValue(record["x"]), numberValue(record["y"]), numberValue(record["z"]); x != nil || y != nil || z != nil {
		out["coords"] = map[string]any{"x": x, "y": y, "z": z}
	}
	if v := first(record, "body_count", "bodyCount"); v != nil {
		out["bodyCount"] = numberValue(v)
	} else if bodies, ok := record["bodies"].([]any); ok {
		out["bodyCount"] = len(bodies)
	}
	if v := numberValue(record["population"]); v != nil {
		out["population"] = v
	}
	if v := stringValue(first(record, "mainStar", "main_star", "main_star_type")); v != "" {
		out["mainStar"] = v
	}
	if v := first(record, "needsPermit", "needs_permit"); v != nil {
		out["needsPermit"] = v
	}
	if v := stringValue(first(record, "updateTime", "updated_at", "updatedAt")); v != "" {
		out["updateTime"] = v
	}
	return out
}

func first(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case float64:
		if x == float64(uint64(x)) {
			return strconv.FormatUint(uint64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func numberValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		if f, err := x.Float64(); err == nil {
			return f
		}
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	}
	return nil
}
