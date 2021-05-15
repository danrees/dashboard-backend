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
	TTL        time.Duration
}

var DEBUG = false

var DEBUG_LOG = log.New(os.Stderr, "DEBUG: ", log.LstdFlags|log.Lshortfile)

//var INFO_LOG = log.New(os.Stderr, "INFO: ", log.LstdFlags|log.Lshortfile)
var ERROR_LOG = log.New(os.Stderr, "INFO: ", log.LstdFlags|log.Lshortfile)

type Server struct {
	cal *calendar.Cacher
	w   *weather.Cacher
}

func NewServer(ctx context.Context, wConfig WeatherConfig, cConfig CalendarConfig) (*Server, error) {
	cal, err := calendar.New(ctx, cConfig.APIKey, cConfig.CalendarID)
	if err != nil {
		return nil, err
	}
	w := weather.New(DefaultWeather, wConfig.CityID, wConfig.APIKey).WithCache(wConfig.TTL)
	server := Server{
		cal: cal.WithCache(cConfig.TTL),
		w:   w,
	}
	return &server, nil
}

func (s *Server) getWeather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "unsupported method", http.StatusBadRequest)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
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

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	switch r.Method {
	case http.MethodGet:
		s.getEvents(w, r)
	case http.MethodPost:
		s.saveCalender(w, r)
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
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
		if DEBUG {
			DEBUG_LOG.Println(e.Description)
		}
	}
	if err := json.NewEncoder(w).Encode(ev.Items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

func (s *Server) saveCalender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		return
	}
	ev := new(calendar.Event)
	if err := json.NewDecoder(r.Body).Decode(ev); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	saved, err := s.cal.Save(r.Context(), ev)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(saved); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
		ERROR_LOG.Printf("%s is not a valid duration, using default 30 minutes", ttlString)
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
	calendarTTLString, ok := os.LookupEnv("CALENDAR_TTL")
	if !ok {
		calendarTTLString = "30m"
	}
	calendarTTL, err := time.ParseDuration(calendarTTLString)
	if err != nil {
		ERROR_LOG.Printf("%s is not a valid duration, using default 30m", calendarTTLString)
		calendarTTL = 30 * time.Minute
	}
	server, err := NewServer(ctx,
		WeatherConfig{
			APIKey: apiKey,
			CityID: cityID,
			TTL:    ttl,
		}, CalendarConfig{
			APIKey:     calenderAPI,
			CalendarID: calendarID,
			TTL:        calendarTTL,
		})
	if err != nil {
		ERROR_LOG.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/weather", server.getWeather)
	mux.HandleFunc("/api/calendar", server.calendar)
	log.Println("Starting server on port 8000")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		panic(err)
	}
}
