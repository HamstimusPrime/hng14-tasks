-- name: CreateUserTable :one
CREATE TABLE users(
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    gender TEXT NOT NULL,
    gender_probability FLOAT NOT NULL,
    sample_size INT NOT NULL,
    age INT NOT NULL,
    age_group TEXT NOT NULL,
    country_id TEXT NOT NULL,
    country_probability FLOAT NOT NULL,
    created_at TIMESTAMP
);