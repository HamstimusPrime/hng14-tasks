package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hng_task_03/internal/database"
	"log"
	"net/http"

	"github.com/google/uuid"
)

func handlerCreateProfile(w http.ResponseWriter, r *http.Request, q *database.Queries) {

	w.Header().Set("Access-Control-Allow-Origin", "*")

	reqBody, err := parseReqBody(r, requestBody{})
	if err != nil {
		log.Printf("unable to parse request body, err: %v\n", err)
		respondWithError(w, 502, " Upstream or server failure")
		return
	}

	if reqBody.Name == "" {
		respondWithError(w, 422, " Unprocessable Entity: Invalid type")
		return
	}

	var dbUser database.User
	var createUserObj userProfile

	dbUser, err = q.GetProfileByName(context.Background(), reqBody.Name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// continue to create profile with name if name not found in DB
			log.Println("user does not exist, continuing...")
		} else {
			// Real error — stop execution
			log.Printf("error fetching profile by name err: %v", err)
			respondWithError(w, 500, "internal server Error")
			return
		}
	}

	if dbUser.Name == reqBody.Name {
		log.Printf("duplicate entry! entry with name: {%v} already exists!!\n", reqBody.Name)

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

		respondWithJSON(w, createUserObj, 200)
		return
	}

	//--- fetch genderizeAPI ---
	genderizeData, err := fetchDataFromAPI[GenderizeResponse](GENDERIZE_API_URL, reqBody.Name, w)
	if err != nil {
		return
	}
	//if Genderize returns gender: null or count: 0 → return 502, do not store
	if (genderizeData.Gender == "") || (genderizeData.Count == 0) {
		respondWithError(w, 502, fmt.Sprintf("%v returned an invalid response", GENDERIZE_API_URL))
		return
	}

	//--- fetch agifyAPI ---
	agifyData, err := fetchDataFromAPI[AgifyResponse](AGIFY_API_URL, reqBody.Name, w)
	if err != nil {
		return
	}
	// If Agify returns age: null → return 502, do not store
	if agifyData.Age == 0 {
		respondWithError(w, 502, fmt.Sprintf("%v returned an invalid response", AGIFY_API_URL))
		return
	}

	//--- fetch nationalizeAPI ---
	nationalizeData, err := fetchDataFromAPI[NationalizeResponse](NATIONALIZE_API_URL, reqBody.Name, w)
	if err != nil {
		return
	}
	// If Nationalize returns no country data → return 502, do not store
	if len(nationalizeData.Country) == 0 {
		respondWithError(w, 502, fmt.Sprintf("%v returned an invalid response", AGIFY_API_URL))
		return
	}

	// Nationality: pick the country with
	// the highest probability from the Nationalize response

	profile := database.CreateProfileParams{
		ID:                 uuid.New(),
		Name:               genderizeData.Name,
		Gender:             genderizeData.Gender,
		GenderProbability:  genderizeData.Probability,
		SampleSize:         int32(genderizeData.Count),
		Age:                int32(agifyData.Age),
		AgeGroup:           ageGroupFromAgify(agifyData.Age),
		CountryID:          getTopCountry(nationalizeData.Country).CountryID,
		CountryProbability: getTopCountry(nationalizeData.Country).Probability,
	}

	dbUser, err = q.CreateProfile(context.Background(), profile)
	if err != nil {
		log.Printf("error creating profile, error: %v", err)
		respondWithError(w, 500, "internal server Error")
		return
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

	respondWithJSON(w, createUserObj, 201)
	return
}

func handlerGetProfileWithID(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	userInput := r.PathValue("id")
	id, err := uuid.Parse(userInput)
	if err != nil {
		log.Printf("error, ID: %v is not a valid UUID", userInput)
		respondWithError(w, 422, "Unprocessable Entity: Invalid type")
		return
	}
	log.Printf("profile id with value: %v is a valid UUID\n", userInput)

	profileFromDB, err := q.GetProfileByID(context.Background(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No user found
			log.Println("profile does not exist")
			respondWithError(w, 404, "Not Found: Profile not found")
			return
		} else {
			// Real error — stop execution
			log.Printf("error fetching user by name err: %v", err)
			respondWithError(w, 500, "internal server Error")
			return
		}
	}

	var profileObj userProfile
	profileObj.Status = "success"
	profileObj.Data = userData{
		ID:                 profileFromDB.ID,
		Name:               profileFromDB.Name,
		Gender:             profileFromDB.Gender,
		GenderProbability:  profileFromDB.GenderProbability,
		SampleSize:         int(profileFromDB.SampleSize),
		Age:                int(profileFromDB.Age),
		AgeGroup:           profileFromDB.AgeGroup,
		CountryID:          profileFromDB.CountryID,
		CountryProbability: profileFromDB.CountryProbability,
		CreatedAt:          profileFromDB.CreatedAt.Time,
	}

	respondWithJSON(w, profileObj, 200)
	return

}

func handlerGetProfiles(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	page := parseIntParam(r, "page", 1)
	limit := parseIntParam(r, "limit", 10)
	if limit > 50 {
		limit = 50
	}
	offset := (page - 1) * limit

	//initialize filtered profiles and first assign default values to it
	filter := database.ProfileFilter{
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	// get accepted queries from request if provided, but default to
	// empty string or zero value if not provided. Use them to build
	//the ProfileFilter struct
	if gender := r.URL.Query().Get("gender"); gender != "" {
		filter.Gender = &gender
	}
	if countryID := r.URL.Query().Get("country_id"); countryID != "" {
		filter.CountryID = &countryID
	}
	if ageGroup := r.URL.Query().Get("age_group"); ageGroup != "" {
		filter.AgeGroup = &ageGroup
	}
	if minAge := parseIntParam(r, "min_age", 0); minAge > 0 {
		v := int32(minAge)
		filter.MinAge = &v
	}
	if maxAge := parseIntParam(r, "max_age", 0); maxAge > 0 {
		v := int32(maxAge)
		filter.MaxAge = &v
	}

	// map of all possible parameters that we could sort our DB columns by
	allowedSortCols := map[string]bool{
		"name": true, "age": true, "gender": true,
		"country_id": true, "gender_probability": true,
		"country_probability": true, "created_at": true,
	}

	//get sortby and order filter value if provided
	if sortBy := r.URL.Query().Get("sort_by"); allowedSortCols[sortBy] {
		filter.SortBy = sortBy
	}
	//convert values for SQL Query compatibility
	order := r.URL.Query().Get("order")
	if order == "desc" || order == "DESC" {
		filter.Order = "DESC"
	} else {
		filter.Order = "ASC"
	}

	//get the total number of profiles that match all our request from the DB
	total, err := q.GetFilteredProfileCount(context.Background(), filter)
	if err != nil {
		log.Printf("error counting profiles: %v", err)
		respondWithError(w, 500, "internal server error")
		return
	}

	profilesFromDB, err := q.GetFilteredProfiles(context.Background(), filter)
	if err != nil {
		log.Printf("error fetching profiles: %v", err)
		respondWithError(w, 500, "internal server error")
		return
	}

	var profiles []allProfiilesData
	for _, p := range profilesFromDB {
		profiles = append(profiles, allProfiilesData{
			ID:                 p.ID,
			Name:               p.Name,
			Gender:             p.Gender,
			GenderProbability:  p.GenderProbability,
			Age:                int(p.Age),
			AgeGroup:           p.AgeGroup,
			CountryID:          p.CountryID,
			CountryName:        countryNameFromCode(p.CountryID),
			CountryProbability: p.CountryProbability,
			CreatedAt:          p.CreatedAt.Time,
		})
	}

	respondWithJSON(w, allProfiiles{
		Status: "success",
		Page:   page,
		Limit:  limit,
		Total:  total,
		Data:   profiles,
	}, 200)
}

func handlerDeleteProfileWithID(w http.ResponseWriter, r *http.Request, q *database.Queries) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	userInput := r.PathValue("id")
	id, err := uuid.Parse(userInput)
	if err != nil {
		errorMsg := fmt.Sprintf("error, ID: %v is not a valid UUID", userInput)
		respondWithError(w, 400, errorMsg)
		return
	}

	err = q.DeleteProfileByID(context.Background(), id)
	if err != nil {
		respondWithError(w, 500, "internal server Error")
		return
	}
	w.WriteHeader(204)

}
