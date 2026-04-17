package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
)

var AGIFY_API_URL string = "https://api.agify.io/"
var GENDERIZE_API_URL string = "https://api.genderize.io/"
var NATIONALIZE_API_URL string = "https://api.nationalize.io/"

func respondWithError(w http.ResponseWriter, statusCode int, errMsg string) {
	w.WriteHeader(statusCode)
	errObj := ErrorObject{Status: "error", Message: errMsg}
	errJSON, _ := json.Marshal(errObj)
	w.Header().Set("Content-Type", "application/json")
	w.Write(errJSON)
	return
}

func respondWithJSON(w http.ResponseWriter, resTemplate interface{}, HTTPstatus int) {
	resJSON, err := json.Marshal(resTemplate)
	if err != nil {
		log.Fatal("unable to parse response JSON")
	}
	w.Header().Set("Content-Type", "json/plain; charset=utf-8")
	w.WriteHeader(HTTPstatus)
	w.Write([]byte(resJSON))
}

func ageGroupFromAgify(age int) string {
	//Age group from Agify: 0–12 → child, 13–19 → teenager, 20–59 → adult, 60+ → senior
	if (age >= 0) && (age <= 12) {
		return "child"
	}
	if (age >= 13) && (age <= 19) {
		return "teenager"
	}
	if (age >= 20) && (age <= 59) {
		return "adult"
	}
	if age >= 60 {
		return "senior"
	}
	return ""

}

func getTopCountry(countries []CountryData) CountryData {
	if len(countries) == 0 {
		return CountryData{}
	}

	sort.Slice(countries, func(i, j int) bool {
		return countries[i].Probability > countries[j].Probability
	})
	//round off country probability to 2 decimal places
	countries[0].Probability = roundTo(countries[0].Probability, 2)
	return countries[0]
}

func roundTo(num float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(num*factor) / factor
}

func fetchDataFromAPI[T any](apiURL string, params string, w http.ResponseWriter) (T, error) {

	var result T

	fullURLPath := fmt.Sprintf("%v?name=%v", apiURL, params)
	log.Printf("fetching data from url: %v...\n", fullURLPath)
	r, err := http.Get(fullURLPath)
	if err != nil {
		msg := fmt.Sprintf("%v returned an invalid response", apiURL)
		respondWithError(w, r.StatusCode, msg)
		return result, errors.New(msg)
	}
	log.Printf("fetch from %v complete!\n", fullURLPath)

	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&result)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		msg := "Upstream or server failure"
		respondWithError(w, r.StatusCode, msg)
		return result, errors.New(msg)
	}
	return result, nil
}

func parseReqBody(req *http.Request, format requestBody) (requestBody, error) {
	if err := json.NewDecoder(req.Body).Decode(&format); err != nil {
		return requestBody{}, err
	}
	return format, nil
}
