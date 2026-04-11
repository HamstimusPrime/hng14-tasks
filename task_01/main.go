package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func main() {
	// Register the handler function for the "/" path
	http.HandleFunc("/api/classify", genderReqHandler)
	handler := http.TimeoutHandler(
		http.DefaultServeMux,
		500*time.Millisecond,
		"request timed out",
	)

	// Start the server on port 8080
	fmt.Println("Server starting on :8080...")
	http.ListenAndServe(":8080", handler)
}

//--- logic ---

type GenderResponse struct {
	Count       int     `json:"count"`
	Name        string  `json:"name"`
	Gender      string  `json:"gender"`
	Probability float32 `json:"probability"`
}

type ErrorObject struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}

type Response struct {
	Status string `json:"status"`
	Data   Data   `json:"data"`
}

type Data struct {
	Name        string    `json:"name"`
	Gender      string    `json:"gender"`
	Probability float64   `json:"probability"`
	SampleSize  int       `json:"sample_size"`
	IsConfident bool      `json:"is_confident"`
	ProcessedAt time.Time `json:"processed_at"`
}

type timeoutHandler struct{}

func (h timeoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusRequestTimeout)
}

func genderReqHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	param := r.URL.Query()
	err, paramObj, name := validateParam(param)
	if err != nil {
		fmt.Printf("error is : %v\n", err)
		respondWithError(w, paramObj.StatusCode, paramObj.Message)
		return
	}

	fullURL := fmt.Sprintf("https://api.genderize.io?name=%v", name)
	fmt.Printf("making request to %v...\n", fullURL)

	genderRes, err := http.Get(fullURL)
	if err != nil {
		fmt.Println("Error creating request:", err)
		msg := "Upstream or server failure"
		respondWithError(w, 500, msg)
		return
	}

	defer genderRes.Body.Close()
	var genderReseObj GenderResponse
	decoder := json.NewDecoder(genderRes.Body)
	if err := decoder.Decode(&genderReseObj); err != nil {
		fmt.Printf("error decoding response body. error: %v", err)
		msg := "Upstream or server failure"
		respondWithError(w, 501, msg)
		return
	}

	if (genderReseObj.Gender == "null") || (genderReseObj.Count == 0) {
		msg := "No prediction available for the provided name"
		respondWithError(w, 501, msg)
		return
	}

	responseObj := Response{
		Status: "success",
		Data: Data{
			Name:        genderReseObj.Name,
			Gender:      genderReseObj.Gender,
			Probability: float64(genderReseObj.Probability),
			SampleSize:  genderReseObj.Count,
			IsConfident: isConfident(genderReseObj.Probability, genderReseObj.Count),
			ProcessedAt: time.Now().UTC(),
		},
	}

	dat, err := json.Marshal(responseObj)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)

		respondWithError(w, 500, "Upstream or server failure")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	errObj := ErrorObject{Status: "error", Message: message}
	errJSON, _ := json.Marshal(errObj)
	w.Header().Set("Content-Type", "application/json")
	w.Write(errJSON)
	return
}

func isConfident(probability float32, sampleSize int) bool {
	return (probability >= 0.7) && (sampleSize >= 100)
}

func validateParam(param url.Values) (error, ErrorObject, string) {
	nameParam := param.Get("name")
	if nameParam == "" {
		msg := "Bad Request: Missing or empty name parameter"
		return fmt.Errorf(msg), ErrorObject{"error", msg, 400}, ""
	}

	_, err := strconv.Atoi(nameParam)
	if err != nil {
		return nil, ErrorObject{"", "", 200}, nameParam
	}

	msg := fmt.Sprintf("Unprocessable Entity: %v is not a string", nameParam)
	return fmt.Errorf(msg), ErrorObject{"error", msg, 422}, ""

}
