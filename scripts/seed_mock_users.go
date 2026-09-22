package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set. Please run this inside the kubernetes pod.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	users := []struct {
		Name  string
		Phone string
	}{
		{"Amit Sharma", "+919876543210"},
		{"Neha Gupta", "+919876543211"},
		{"Rahul Verma", "+919876543212"},
		{"Priya Singh", "+919876543213"},
		{"Karan Patel", "+919876543214"},
	}

	for _, u := range users {
		id := uuid.New()
		astraID := fmt.Sprintf("A-%d", time.Now().UnixNano()%100000)
		_, err := pool.Exec(ctx, `
			INSERT INTO users (id, astra_user_id, phone_number, name, wants_rm)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT (phone_number) DO NOTHING
		`, id, astraID, u.Phone, u.Name)
		if err != nil {
			log.Printf("Failed to insert %s: %v", u.Name, err)
		} else {
			log.Printf("Inserted mock user: %s", u.Name)
		}
	}
	
	fmt.Println("Mock users seeded successfully!")
	fmt.Println("Kubernetes will automatically assign these users to the RMs next time the backend pod restarts.")
}
