package supa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

func New() *Client {
	return &Client{
		baseURL: os.Getenv("SUPABASE_URL") + "/rest/v1",
		key:     os.Getenv("SUPABASE_SERVICE_KEY"),
		http:    &http.Client{},
	}
}

func (c *Client) do(method, path string, body interface{}, prefer string) ([]byte, int, error) {
	var rb io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rb = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+"/"+path, rb)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *Client) Select(table, query string, result interface{}) error {
	data, status, err := c.do("GET", table+"?"+query, nil, "")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("supabase %d: %s", status, string(data))
	}
	return json.Unmarshal(data, result)
}

func (c *Client) Insert(table string, body interface{}, result interface{}) error {
	data, status, err := c.do("POST", table, body, "return=representation")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("supabase insert %d: %s", status, string(data))
	}
	if result != nil {
		return json.Unmarshal(data, result)
	}
	return nil
}

func (c *Client) Update(table, query string, body interface{}) error {
	data, status, err := c.do("PATCH", table+"?"+query, body, "return=minimal")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("supabase update %d: %s", status, string(data))
	}
	return nil
}

func (c *Client) Delete(table, query string) error {
	data, status, err := c.do("DELETE", table+"?"+query, nil, "")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("supabase delete %d: %s", status, string(data))
	}
	return nil
}

func (c *Client) Upsert(table, onConflict string, body interface{}) error {
	data, status, err := c.do("POST", table+"?on_conflict="+onConflict, body, "resolution=merge-duplicates,return=minimal")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("supabase upsert %d: %s", status, string(data))
	}
	return nil
}
