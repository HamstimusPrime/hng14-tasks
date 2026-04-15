package main

import (
	"encoding/json"
	"net/http"
)

func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	errObj := ErrorObject{Status: "error", Message: message}
	errJSON, _ := json.Marshal(errObj)
	w.Header().Set("Content-Type", "application/json")
	w.Write(errJSON)
	return
}
