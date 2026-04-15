package main

import (
	"fmt"
	"hng_task_02/internal/database"
	"net/http"
)

func handlerCreateProfile(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	fmt.Fprint(w, "from create profile handler!!")
	fmt.Println("create profile!!")
}
func handlerGetProfileWithID(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	fmt.Fprint(w, "from get profile with ID handler!")
	fmt.Println("get profile with ID!!")
}
func handlerGetUsers(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	fmt.Fprint(w, "from get all profiles handler!!")
	fmt.Println("get all profiles!!")
}
func handlerDeleteProfile(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	fmt.Fprint(w, "from delete profile handler!!")
	fmt.Println("delete profile!!")
}
