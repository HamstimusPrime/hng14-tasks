package main

import (
	"time"

	"github.com/google/uuid"
)

type allProfiiles struct {
	Status string             `json:"status"`
	Count  int                `json:"count"`
	Data   []allProfiilesData `json:"data"`
}

type allProfiilesData struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Gender    string    `json:"gender"`
	Age       int       `json:"age"`
	AgeGroup  string    `json:"age_group"`
	CountryID string    `json:"country_id"`
}

type userProfile struct {
	Status  string   `json:"status"`
	Message string   `json:"message,omitempty"`
	Data    userData `json:"data"`
}

type userData struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Gender             string    `json:"gender"`
	GenderProbability  float64   `json:"gender_probability"`
	SampleSize         int       `json:"sample_size"`
	Age                int       `json:"age"`
	AgeGroup           string    `json:"age_group"`
	CountryID          string    `json:"country_id"`
	CountryProbability float64   `json:"country_probability"`
	CreatedAt          time.Time `json:"created_at"`
}

type ErrorObject struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}

type NationalizeResponse struct {
	Count   int           `json:"count"`
	Name    string        `json:"name"`
	Country []CountryData `json:"country"`
}

type CountryData struct {
	CountryID   string  `json:"country_id"`
	Probability float64 `json:"probability"`
}

type AgifyResponse struct {
	Count int    `json:"count"`
	Name  string `json:"name"`
	Age   int    `json:"age"`
}

type GenderizeResponse struct {
	Count       int     `json:"count"`
	Name        string  `json:"name"`
	Gender      string  `json:"gender"`
	Probability float64 `json:"probability"`
}

type requestBody struct {
	Name string `json:"name"`
}
