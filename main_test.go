package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWeatherHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/api/weather", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(weatherHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var weatherData []WeatherData
	if err := json.Unmarshal(rr.Body.Bytes(), &weatherData); err != nil {
		t.Errorf("Failed to parse response JSON: %v", err)
	}

	if len(weatherData) == 0 {
		t.Error("Expected non-empty weather data")
	}

	for _, data := range weatherData {
		if data.Time == "" {
			t.Error("Expected non-empty time field")
		}
	}
}

func TestHelloHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(helloHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var response HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response JSON: %v", err)
	}

	if response.Message != "Hello World!" {
		t.Errorf("Expected message 'Hello World!', got %v", response.Message)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got %v", response.Status)
	}
}