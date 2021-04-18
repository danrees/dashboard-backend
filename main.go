package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/danrees/pi-backend/pkg/calendar"
	"github.com/danrees/pi-backend/pkg/weather"
	"github.com/joho/godotenv"
)

const DefaultWeather = "https://api.openweathermap.org"

type WeatherConfig struct {
	APIKey string
	CityID string
	URL    string
	TTL    time.Duration
}

type CalendarConfig struct {
	APIKey     string
	CalendarID string
}

var DEBUG = false

var DEBUG_LOG = log.New(os.Stderr, "DEBUG: ", log.LstdFlags|log.Lshortfile)
var INFO_LOG = log.New(os.Stderr, "INFO: ", log.LstdFlags|log.Lshortfile)
var ERROR_LOG = log.New(os.Stderr, "INFO: ", log.LstdFlags|log.Lshortfile)

type Server struct {
	cal *calendar.Cacher
	w   *weather.Cacher
}

func NewServer(ctx context.Context, wConfig WeatherConfig, cConfig CalendarConfig) (*Server, error) {
	cal, err := calendar.WithCache(calendar.New(ctx, cConfig.APIKey, cConfig.CalendarID))
	if err != nil {
		return nil, err
	}
	w := weather.WithCache(weather.New(DefaultWeather, wConfig.CityID, wConfig.APIKey), 2*time.Minute)
	server := Server{
		cal: cal,
		w:   w,
	}
	return &server, nil
}

func (s *Server) getWeather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "unsupported method", http.StatusBadRequest)
		return
	}
	wr, err := s.w.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(wr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, fmt.Sprintf("%v usupported method", r.Method), http.StatusMethodNotAllowed)
		return
	}
	ev, err := s.cal.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, e := range ev.Items {
		DEBUG_LOG.Println(e.Description)
	}
}

func (s *Server) getCalendars(w http.ResponseWriter, r *http.Request) {
	cl, err := s.cal.Calendars(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, cal := range cl.Items {
		DEBUG_LOG.Println((cal.Id))
	}
}

func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {

}

func init() {
	if err := godotenv.Load(); err != nil {
		ERROR_LOG.Println("unable to load env file: ", err.Error())
	}
	_, ok := os.LookupEnv("DEBUG")
	if ok {
		DEBUG = true
	}
}

func main() {
	if DEBUG {
		fmt.Println("debug loggin")
	}
	ctx := context.Background()

	apiKey, ok := os.LookupEnv("WEATHER_API_KEY")
	if !ok {
		ERROR_LOG.Fatal("You must provide an open weather map api key")
	}
	cityID, ok := os.LookupEnv("WEATHER_CITY_ID")
	if !ok {
		ERROR_LOG.Fatal("You must provide an open weather map city id")
	}
	ttlString, ok := os.LookupEnv("WEATHER_CACHE_TTL")
	if !ok {
		ttlString = "30m"
	}

	ttl, err := time.ParseDuration(ttlString)
	if err != nil {
		ttl = 30 * time.Minute
	}
	calenderAPI, ok := os.LookupEnv("CALENDAR_API_KEY")
	if !ok {
		ERROR_LOG.Fatal("You must provide a google calendar api key")
	}
	calendarID, ok := os.LookupEnv("CALENDAR_ID")
	if !ok {
		ERROR_LOG.Fatal("You must provide a calendar id")
	}
	server, err := NewServer(ctx,
		WeatherConfig{
			APIKey: apiKey,
			CityID: cityID,
			TTL:    ttl,
		}, CalendarConfig{
			APIKey:     calenderAPI,
			CalendarID: calendarID,
		})
	if err != nil {
		ERROR_LOG.Fatal(err)
	}
	br := Broker{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/weather", server.getWeather)
	mux.HandleFunc("/api/calendar", server.getEvents)
	mux.Handle("/subscribe", &br)
	log.Println("Starting server on port 8000")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		panic(err)
	}
}

type Broker struct {
	clients        map[chan *weather.Weather]bool
	newClients     chan chan *weather.Weather
	defunctClients chan chan *weather.Weather
	msg            chan *weather.Weather
}

func (b *Broker) Start() error {
	go func() {
		for {
			select {
			case c := <-b.newClients:
				b.clients[c] = true
			case c := <-b.defunctClients:
				delete(b.clients, c)
				close(c)
			case m := <-b.msg:
				for k := range b.clients {
					k <- m
				}
			}
		}
	}()

	return nil
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "events not supported", http.StatusInternalServerError)
		return
	}
	messageChan := make(chan *weather.Weather)

	b.newClients <- messageChan

	notify := w.(http.CloseNotifier).CloseNotify()
	go func() {
		<-notify
		b.defunctClients <- messageChan

	}()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("Transfer-Encoding", "chunked")

	for {
		msg, open := <-messageChan
		if !open {
			break
		}
		if err := json.NewEncoder(w).Encode(msg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f.Flush()
	}
}
