package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"mikan/internal/panel/growth"
)

// The referral programme keeps its data in the addon's KV store and gives days through the
// Panel API's grant call (permissions kv and users:grant); both are plain calls of the Panel
// API, not the bot's RPC, so they live apart from it.

type growthStore struct{ r *Remote }
type growthBank struct{ r *Remote }

var (
	_ growth.Store   = growthStore{}
	_ growth.Granter = growthBank{}
)

// Growth is the referral programme over this Remote.
func (r *Remote) Growth() *growth.Service {
	return &growth.Service{Store: growthStore{r}, Bank: growthBank{r}}
}

func (r *Remote) raw(ctx context.Context, method, path string, q url.Values, body []byte, hdr map[string]string) (*http.Response, error) {
	u := r.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return r.hc.Do(req)
}

func (s growthStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	resp, err := s.r.raw(ctx, http.MethodGet, "/kv/"+key, nil, nil, nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return b, true, err
	case http.StatusNotFound:
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("kv get: HTTP %d", resp.StatusCode)
}

func (s growthStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	resp, err := s.r.raw(ctx, http.MethodPut, "/kv/"+key, url.Values{"if_absent": {"1"}}, value, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return true, nil
	case http.StatusConflict:
		return false, nil
	}
	return false, fmt.Errorf("kv put: HTTP %d", resp.StatusCode)
}

func (s growthStore) Put(ctx context.Context, key string, value []byte) error {
	resp, err := s.r.raw(ctx, http.MethodPut, "/kv/"+key, nil, value, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("kv put: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (b growthBank) Grant(ctx context.Context, userID, days int64, reason, key string) error {
	body, _ := json.Marshal(map[string]any{"days": days, "reason": reason, "key": key})
	resp, err := b.r.raw(ctx, http.MethodPost, "/users/"+strconv.FormatInt(userID, 10)+"/grant", nil, body,
		map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var e struct{ Code, Message string }
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&e)
	return errors.New("grant: " + strconv.Itoa(resp.StatusCode) + " " + e.Code)
}
