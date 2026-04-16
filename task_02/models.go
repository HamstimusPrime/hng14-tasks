package main

import (
	"time"

	"github.com/google/uuid"
)

type allProfiiles struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
	Data   struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Gender    string `json:"gender"`
		Age       int    `json:"age"`
		AgeGroup  string `json:"age_group"`
		CountryID string `json:"country_id"`
	} `json:"data"`
}

type userProfile struct {
	Status  string   `json:"status"`
	Message string   `json:"message,omitempty"`
	Data    userData `json:"data"`
}

type userData struct {
	ID                 uuid.UUID
	Name               string
	Gender             string
	GenderProbability  float64
	SampleSize         int
	Age                int
	AgeGroup           string
	CountryID          string
	CountryProbability float64
	CreatedAt          time.Time
}

type ErrorObject struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}

type NationalizeResponse struct {
	Count   int    `json:"count"`
	Name    string `json:"name"`
	Country []struct {
		CountryID   string  `json:"country_id"`
		Probability float64 `json:"probability"`
	} `json:"country"`
}

type AgifyResponse struct {
	Count int    `json:"count"`
	Name  string `json:"name"`
	Age   int    `json:"age"`
}

type GenderizeResponse struct {
	Name        string    `json:"name"`
	Gender      string    `json:"gender"`
	Probability float64   `json:"probability"`
	SampleSize  int       `json:"sample_size"`
	IsConfident bool      `json:"is_confident"`
	ProcessedAt time.Time `json:"processed_at"`
}
