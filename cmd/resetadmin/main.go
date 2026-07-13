package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/sam33339999/wikibuild/internal/auth"
)

func main() {
	_ = godotenv.Load()
	user := os.Getenv("WIKIBUILD_ADMIN_USER")
	pass := os.Getenv("WIKIBUILD_ADMIN_PASS")
	dbURL := os.Getenv("DATABASE_URL")
	if user == "" || pass == "" || dbURL == "" {
		log.Fatal("need DATABASE_URL, WIKIBUILD_ADMIN_USER, WIKIBUILD_ADMIN_PASS")
	}
	hash, err := auth.NewPasswordHasher().Hash(pass)
	if err != nil {
		log.Fatal(err)
	}
	if len(hash) < 20 {
		log.Fatal("hash too short")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	tag, err := pool.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE username = $2`, hash, user)
	if err != nil {
		log.Fatal(err)
	}
	if tag.RowsAffected() == 0 {
		_, err = pool.Exec(ctx,
			`INSERT INTO users (username, password_hash) VALUES ($1, $2)`, user, hash)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("created user:", user)
		return
	}
	fmt.Println("updated password for:", user)

	// verify
	var got string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE username=$1`, user).Scan(&got); err != nil {
		log.Fatal(err)
	}
	if err := auth.NewPasswordHasher().Compare(got, pass); err != nil {
		log.Fatal("verify failed:", err)
	}
	fmt.Println("verified: .env password matches DB")
}
