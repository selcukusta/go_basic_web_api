package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultPort           = "8080"
	defaultReadTimeout    = 15 * time.Second
	defaultWriteTimeout   = 15 * time.Second
	defaultIdleTimeout    = 60 * time.Second
	defaultMaxHeaderBytes = 1 << 20 // 1 MB
	defaultRequestTimeout = 10 * time.Second
)

var (
	// Simple in-memory rate limiter
	requestCounts = make(map[string]int)
	lastReset     = time.Now()
	maxRequests   = 100 // Max requests per minute per IP
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

// securityHeaders adds essential security headers to all responses
func securityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		// CORS headers - restrict to specific origins in production
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// rateLimiter implements simple IP-based rate limiting
func rateLimiter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()

		// Reset counter every minute
		if now.Sub(lastReset) > time.Minute {
			requestCounts = make(map[string]int)
			lastReset = now
		}

		clientIP := r.RemoteAddr
		requestCounts[clientIP]++

		if requestCounts[clientIP] > maxRequests {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}

// requestLogger logs incoming requests
func requestLogger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Log request details
		log.Printf("[%s] %s %s %s",
			start.Format("2006-01-02 15:04:05"),
			r.Method,
			r.URL.Path,
			r.RemoteAddr,
		)

		next(w, r)

		// Log response time
		log.Printf("Response time: %v", time.Since(start))
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate request path
	if r.URL.Path != "/api/health" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Validate request size
	if r.ContentLength > 0 {
		http.Error(w, "Request body not allowed", http.StatusBadRequest)
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

	// Create context with timeout for external API call
	ctx, cancel := context.WithTimeout(r.Context(), defaultRequestTimeout)
	defer cancel()

	// Fetch data from Open-Meteo API with context
	url := "https://api.open-meteo.com/v1/forecast?latitude=41.05&longitude=28.72&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		http.Error(w, "Failed to create weather request", http.StatusInternalServerError)
		return
	}

	// Use a custom HTTP client with timeout
	client := &http.Client{
		Timeout: defaultRequestTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("Weather API request timed out")
			http.Error(w, "Weather service timeout", http.StatusGatewayTimeout)
			return
		}
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
	// Add cache control header - cache for 5 minutes
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(weatherData); err != nil {
		log.Printf("Error encoding weather response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func main() {
	// Get port from environment variable or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// Apply middleware chain to handlers
	healthHandler := securityHeaders(rateLimiter(requestLogger(helloHandler)))
	weatherHandler := securityHeaders(rateLimiter(requestLogger(weatherHandler)))

	http.HandleFunc("/api/health", healthHandler)
	http.HandleFunc("/api/weather", weatherHandler)

	// Create server with timeouts
	server := &http.Server{
		Addr:           ":" + port,
		ReadTimeout:    defaultReadTimeout,
		WriteTimeout:   defaultWriteTimeout,
		IdleTimeout:    defaultIdleTimeout,
		MaxHeaderBytes: defaultMaxHeaderBytes,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server stopped")
}