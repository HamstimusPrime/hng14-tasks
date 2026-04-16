-- name: CreateUser :one
INSERT INTO users(id, name, gender, gender_probability, sample_size, age,age_group, country_id, country_probability, created_at )
VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    NOW()
)RETURNING *;

-- name: GetUserByName :one
 SELECT * FROM users
WHERE name = $1;
