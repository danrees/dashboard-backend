package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/danrees/pi-backend/pkg/weather"
	"github.com/joho/godotenv"
)

var DEBUG = false

var DEBUG_LOG = log.New(os.Stderr, "DEBUG: ", log.LstdFlags)
var INFO_LOG = log.New(os.Stderr, "INFO: ", log.LstdFlags)

type Cache struct {
	mu     *sync.Mutex
	item   *weather.Weather
	stamp  *time.Time
	client *weather.Client
	ttl    time.Duration
}

func (c *Cache) get() (*weather.Weather, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if (c.item == nil || c.stamp == nil) || time.Now().After(c.stamp.Add(c.ttl)) {
		if DEBUG {
			if c.item == nil || c.stamp == nil {
				DEBUG_LOG.Println("Setting weather value for the first time")
			} else {
				DEBUG_LOG.Println("Refresing weather data")
			}
		}
		stamp := time.Now()
		c.stamp = &stamp
		w, err := c.client.Get()
		if err != nil {
			return nil, err
		}
		c.item = w
	} else {
		DEBUG_LOG.Printf("returning weather data from cache, valid for %v", c.ttl-time.Since(*c.stamp))
	}
	return c.item, nil
}

type Server struct {
	cache *Cache
}

func NewServer(cityID, apiKey string, ttl time.Duration) *Server {
	server := Server{
		cache: &Cache{
			mu:     &sync.Mutex{},
			client: weather.New("https://api.openweathermap.org", cityID, apiKey),
			ttl:    ttl,
		},
	}
	return &server
}

func (s *Server) getWeather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "unsupported method", http.StatusBadRequest)
		return
	}
	wr, err := s.cache.get()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(wr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("unable to load env file: ", err.Error())
	}
	apiKey, ok := os.LookupEnv("WEATHER_API_KEY")
	if !ok {
		log.Fatal("You must provide an open weather map api key")
	}
	cityID, ok := os.LookupEnv("WEATHER_CITY_ID")
	if !ok {
		log.Fatal("You must provide an open weather map city id")
	}
	ttlString, ok := os.LookupEnv("WEATHER_CACHE_TTL")
	if !ok {
		ttlString = "30m"
	}
	_, ok = os.LookupEnv("WEATHER_DEBUG")
	if ok {
		DEBUG = true
	}
	ttl, err := time.ParseDuration(ttlString)
	if err != nil {
		ttl = 30 * time.Minute
	}
	server := NewServer(cityID, apiKey, ttl)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/weather", server.getWeather)
	log.Println("Starting server on port 8000")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		panic(err)
	}
}
