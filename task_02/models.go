package main

import "time"

type allProfiiles struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
	Data   []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Gender    string `json:"gender"`
		Age       int    `json:"age"`
		AgeGroup  string `json:"age_group"`
		CountryID string `json:"country_id"`
	} `json:"data"`
}

type userProfile struct {
	Status string `json:"status"`
	Data   struct {
		ID                 string    `json:"id"`
		Name               string    `json:"name"`
		Gender             string    `json:"gender"`
		GenderProbability  float64   `json:"gender_probability"`
		SampleSize         int       `json:"sample_size"`
		Age                int       `json:"age"`
		AgeGroup           string    `json:"age_group"`
		CountryID          string    `json:"country_id"`
		CountryProbability float64   `json:"country_probability"`
		CreatedAt          time.Time `json:"created_at"`
	} `json:"data"`
}

type ErrorObject struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}
