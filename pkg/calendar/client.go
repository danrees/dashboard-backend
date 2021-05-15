package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

var DefaultTTL = 2 * time.Minute

type Event struct {
	Summary string    `json:"summary,omitempty"`
	Date    time.Time `json:"date,omitempty"`
}

type Client struct {
	svc        *calendar.Service
	calendarID string
	debug      *log.Logger
}

type Cacher struct {
	my       *sync.Mutex
	events   *calendar.Events
	cachedAt time.Time
	ttl      time.Duration
	*Client
}

func auth(ctx context.Context, config *oauth2.Config) (*http.Client, error) {
	var err error
	token := &oauth2.Token{}
	f, err := os.Open(".saved-token.json")
	if os.IsNotExist(err) {
		token, err = webAuth(ctx, config)
	} else if err != nil {
		return nil, err
	} else {
		if err := json.NewDecoder(f).Decode(token); err != nil {
			return nil, err
		}
	}
	return config.Client(ctx, token), nil
}

func webAuth(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-string", oauth2.AccessTypeOffline)
	fmt.Println("Go to: ", authURL)
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, err
	}
	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(".saved-token.json")
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(f).Encode(tok); err != nil {
		return nil, err
	}
	return tok, nil
}

func New(ctx context.Context, apiKey string, calendarID string) (*Client, error) {

	b, err := ioutil.ReadFile(".oauth-creds.json")
	if err != nil {
		return nil, err
	}
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope, calendar.CalendarEventsScope)
	if err != nil {
		return nil, err
	}
	cl, err := auth(ctx, config)
	if err != nil {
		return nil, err
	}

	cal, err := calendar.NewService(ctx, option.WithHTTPClient(cl))
	if err != nil {
		return nil, err
	}
	return &Client{
		svc:        cal,
		calendarID: calendarID,
		debug:      log.New(os.Stderr, "DEBUG: ", log.LstdFlags),
	}, nil
}

func (c *Client) WithCache(ttl time.Duration) *Cacher {

	return &Cacher{
		Client: c,
		ttl:    ttl,
		my:     &sync.Mutex{},
	}
}

func (c *Client) Calendars(ctx context.Context) (*calendar.CalendarList, error) {
	cl, err := c.svc.CalendarList.List().
		Context(ctx).
		Do()

	return cl, err
}

func (c *Client) List(ctx context.Context) (*calendar.Events, error) {
	evs, err := c.svc.Events.
		List(c.calendarID).
		SingleEvents(true).
		TimeMin(time.Now().Format(time.RFC3339)).
		ShowDeleted(false).
		TimeMax(time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("could not list calendar events: %w", err)
	}
	return evs, nil
}

func (c *Client) Save(ctx context.Context, ev *Event) (*calendar.Event, error) {
	cl := c.svc.Events.Insert(c.calendarID, &calendar.Event{
		Summary: ev.Summary,
		Start: &calendar.EventDateTime{
			Date: ev.Date.Format("2006-01-02"),
		},
		End: &calendar.EventDateTime{
			Date: ev.Date.Format("2006-01-02"),
		},
	})
	saved, err := cl.
		Context(ctx).
		Do()
	return saved, err
}

func (c *Cacher) List(ctx context.Context) (*calendar.Events, error) {
	c.my.Lock()
	defer c.my.Unlock()
	var err error
	if c.events == nil || !c.cachedAt.Add(c.ttl).After(time.Now()) {
		c.Client.debug.Print("cache miss querying")
		c.events, err = c.Client.List(ctx)
		if err != nil {
			return nil, err
		}
		c.cachedAt = time.Now()
	}
	return c.events, nil
}
