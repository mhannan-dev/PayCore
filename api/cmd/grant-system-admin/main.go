package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"paycore/api/database/migrations"
	"paycore/api/database/seeders"
	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/repositories"
)

func main() {
	email := flag.String("email", "admin@yopmail.com", "Existing account to grant system administrator access")
	flag.Parse()
	*email = strings.ToLower(strings.TrimSpace(*email))
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Database connection failed.")
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		log.Fatal(err)
	}
	if err = seeders.GrantSystemAdmin(ctx, pool, *email); err != nil {
		log.Fatal(err)
	}
	u, err := repositories.NewUserRepository(pool).FindByEmail(ctx, *email)
	if err != nil {
		log.Fatal("Access granted, but verification failed.")
	}
	var total int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM permissions").Scan(&total); err != nil {
		log.Fatal("Permission verification failed.")
	}
	if !u.HasRole("ADMIN") || !u.IsActive || len(u.Permissions) != total {
		log.Fatal("System administrator permission verification failed.")
	}
	log.Printf("System administrator: %s; roles: %v; permissions: %d; active: %t", u.Email, u.Roles, len(u.Permissions), u.IsActive)
}
