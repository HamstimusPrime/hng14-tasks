package main

import (
	"context"
	"fmt"
	"hng_task_02/internal/database"
	"net/http"

	"github.com/google/uuid"
)

func handlerCreateProfile(w http.ResponseWriter, r *http.Request, q *database.Queries) {

	w.Header().Set("Access-Control-Allow-Origin", "*")
	err, errObj, nameParam := validateParam(r.URL.Query())
	if err != nil {
		respondWithError(w, errObj.StatusCode, errObj.Message)
		return
	}

	var dbUser database.User
	var createUserObj userProfile
	dbUser, err = q.GetUserByName(context.Background(), nameParam)
	if err != nil {
		fmt.Println("error fetching user by name")
		return
	}

	if dbUser.Name == nameParam {
		fmt.Printf("duplicate entry! entry with name: {%v} already exists!!\n", nameParam)

		createUserObj.Status = "success"
		createUserObj.Message = "Profile already exists"
		createUserObj.Data = userData{
			ID:                 dbUser.ID,
			Name:               dbUser.Name,
			Gender:             dbUser.Gender,
			GenderProbability:  dbUser.GenderProbability,
			SampleSize:         int(dbUser.SampleSize),
			Age:                int(dbUser.Age),
			AgeGroup:           dbUser.AgeGroup,
			CountryID:          dbUser.CountryID,
			CountryProbability: dbUser.CountryProbability,
			CreatedAt:          dbUser.CreatedAt.Time,
		}

		//---respond with JSON
		respondWithJSON(w, createUserObj, 420)
		return
	}

	//--- fetch genderizeAPI ---
	genderizeData, err := fetchDataFromAPI[GenderizeResponse](GENDERIZE_API_URL, nameParam, w)
	if err != nil {
		return
	}
	//--- fetch agifyAPI ---
	agifyData, err := fetchDataFromAPI[AgifyResponse](AGIFY_API_URL, nameParam, w)
	if err != nil {
		return
	}
	//--- fetch nationalizeAPI ---
	nationalizeData, err := fetchDataFromAPI[NationalizeResponse](NATIONALIZE_API_URL, nameParam, w)
	if err != nil {
		return
	}

	user := database.CreateUserParams{
		ID:                 uuid.New(),
		Name:               genderizeData.Name,
		Gender:             genderizeData.Gender,
		GenderProbability:  genderizeData.Probability,
		SampleSize:         int32(genderizeData.SampleSize),
		Age:                int32(agifyData.Age),
		CountryID:          nationalizeData.Country[0].CountryID,
		CountryProbability: nationalizeData.Country[0].Probability,
	}

	dbUser, err = q.CreateUser(context.Background(), user)
	if err != nil {
		fmt.Printf("error creating user, error: %v", err)
		return
		//respond with internal server error Here!
	}

	createUserObj.Status = "success"
	createUserObj.Data = userData{
		ID:                 dbUser.ID,
		Name:               dbUser.Name,
		Gender:             dbUser.Gender,
		GenderProbability:  dbUser.GenderProbability,
		SampleSize:         int(dbUser.SampleSize),
		Age:                int(dbUser.Age),
		AgeGroup:           dbUser.AgeGroup,
		CountryID:          dbUser.CountryID,
		CountryProbability: dbUser.CountryProbability,
		CreatedAt:          dbUser.CreatedAt.Time,
	}

	respondWithJSON(w, createUserObj, 200)
	return
}

func handlerGetProfileWithID(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	// fmt.Fprint(w, "from get profile with ID handler!")
	// fmt.Println("get profile with ID!!")
	// w.Header().Set("Access-Control-Allow-Origin", "*")
	// err, errObj, nameParam := validateParam(r.URL.Query())
	// if err != nil {
	// 	respondWithError(w, errObj.StatusCode, errObj.Message)
	// }
	//call external genderizeAPI with name Params

}
func handlerGetUsers(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	fmt.Fprint(w, "from get all profiles handler!!")
	fmt.Println("get all profiles!!")
}
func handlerDeleteProfile(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	fmt.Fprint(w, "from delete profile handler!!")
	fmt.Println("delete profile!!")
}
