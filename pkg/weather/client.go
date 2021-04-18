package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

func New(addr, cityID, apiKey string) *Client {
	return &Client{
		addr:   addr,
		apiKey: apiKey,
		cityID: cityID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func WithCache(c *Client, ttl time.Duration) *Cacher {
	return &Cacher{
		Client: c,
		cached: nil,
		ttl:    ttl,
		mu:     &sync.Mutex{},
	}
}

type Client struct {
	client *http.Client
	addr   string
	apiKey string
	cityID string
}

func (c *Client) Get(ctx context.Context) (*Weather, error) {
	addr := fmt.Sprintf("%s/data/2.5/weather", c.addr)

	link, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	params := link.Query()
	params.Add("id", c.cityID)
	params.Add("appid", c.apiKey)
	link.RawQuery = params.Encode()

	//resp, err := http.Get(link.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	w := new(Weather)
	if err := json.NewDecoder(resp.Body).Decode(w); err != nil {
		return nil, err
	}

	return w, nil
}

type Cacher struct {
	mu        *sync.Mutex
	cached    *Weather
	expiresAt time.Time
	ttl       time.Duration
	*Client
}

func (c *Cacher) Get(ctx context.Context) (*Weather, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.cached == nil || time.Now().After(c.expiresAt) {
		c.cached, err = c.Client.Get(ctx)
		if err != nil {
			return nil, err
		}
		c.expiresAt = time.Now().Add(c.ttl)
	}
	return c.cached, nil
}

type Weather struct {
	Coord struct {
		Lon float64 `json:"lon,omitempty"`
		Lat float64 `json:"lat,omitempty"`
	} `json:"coord,omitempty"`
	Weather []struct {
		Main        string `json:"main,omitempty"`
		Description string `json:"description,omitempty"`
		Icon        string `json:"icon,omitempty"`
		ID          int64  `json:"id,omitempty"`
	} `json:"weather,omitempty"`
	Base string `json:"base,omitempty"`
	Main struct {
		Temp      float64 `json:"temp,omitempty"`
		FeelsLike float64 `json:"feels_like,omitempty"`
		TempMin   float64 `json:"temp_min,omitempty"`
		TempMax   float64 `json:"temp_max,omitempty"`
		Pressure  int64   `json:"pressure,omitempty"`
		Humidity  int64   `json:"humidity,omitempty"`
	} `json:"main,omitempty"`
	Visibility int64 `json:"visibility,omitempty"`
	Wind       struct {
		Speed float64 `json:"speed,omitempty"`
		Deg   int64   `json:"deg,omitempty"`
		Gust  float64 `json:"gust,omitempty"`
	} `json:"wind,omitempty"`
	Clouds struct {
		All int `json:"all,omitempty"`
	} `json:"clouds,omitempty"`
	DT  int64 `json:"dt,omitempty"`
	Sys struct {
		Type    int64  `json:"type,omitempty"`
		ID      int64  `json:"id,omitempty"`
		Country string `json:"country,omitempty"`
		Sunrise int64  `json:"sunrise,omitempty"`
		Sunset  int64  `json:"sunset,omitempty"`
	} `json:"sys,omitempty"`
	Timezone int64  `json:"timezone,omitempty"`
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	COD      int64  `json:"cod,omitempty"`
}
