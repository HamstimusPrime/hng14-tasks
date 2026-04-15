-- name: CreateUser :one
INSERT INTO users(id, name, gender, gender_probability, sample_size, age,age_group, country_id, country_probability, created_at )
VALUES(
    gen_random_uuid(),
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    NOW()
)RETURNING *;