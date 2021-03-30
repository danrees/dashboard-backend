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

type Cache struct {
	mu     *sync.Mutex
	item   *weather.Weather
	stamp  *time.Time
	client *weather.Client
}

func (c *Cache) get() (*weather.Weather, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if (c.item == nil || c.stamp == nil) || time.Now().After(c.stamp.Add(30*time.Minute)) {
		stamp := time.Now()
		c.stamp = &stamp
		w, err := c.client.Get()
		if err != nil {
			return nil, err
		}
		c.item = w
	}
	return c.item, nil
}

type Server struct {
	cache *Cache
}

func NewServer(cityID, apiKey string) *Server {
	server := Server{
		cache: &Cache{
			mu:     &sync.Mutex{},
			client: weather.New("https://api.openweathermap.org", cityID, apiKey),
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
	server := NewServer(cityID, apiKey)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/weather", server.getWeather)
	log.Println("Starting server on port 8000")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		panic(err)
	}
}
