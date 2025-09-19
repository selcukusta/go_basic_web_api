package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

type HealthResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type WeatherData struct {
	Time          string  `json:"time"`
	Temperature2m float64 `json:"temperature_2m"`
}

type OpenMeteoResponse struct {
	Hourly struct {
		Time          []string  `json:"time"`
		Temperature2m []float64 `json:"temperature_2m"`
	} `json:"hourly"`
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := HealthResponse{
		Message: "Hello World!",
		Status:  "success",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Fetch data from Open-Meteo API
	url := "https://api.open-meteo.com/v1/forecast?latitude=41.05&longitude=28.72&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m"

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error fetching weather data: %v", err)
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Open-Meteo API returned status: %d", resp.StatusCode)
		http.Error(w, "Weather service unavailable", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		http.Error(w, "Failed to read weather data", http.StatusInternalServerError)
		return
	}

	var openMeteoResp OpenMeteoResponse
	if err := json.Unmarshal(body, &openMeteoResp); err != nil {
		log.Printf("Error parsing weather data: %v", err)
		http.Error(w, "Failed to parse weather data", http.StatusInternalServerError)
		return
	}

	// Transform data into required format
	var weatherData []WeatherData
	for i := 0; i < len(openMeteoResp.Hourly.Time) && i < len(openMeteoResp.Hourly.Temperature2m); i++ {
		weatherData = append(weatherData, WeatherData{
			Time:          openMeteoResp.Hourly.Time[i],
			Temperature2m: openMeteoResp.Hourly.Temperature2m[i],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(weatherData); err != nil {
		log.Printf("Error encoding weather response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func main() {
	http.HandleFunc("/api/health", helloHandler)
	http.HandleFunc("/api/weather", weatherHandler)

	port := ":8080"
	log.Printf("Server starting on port %s", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}