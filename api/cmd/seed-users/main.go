package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
	"paycore/api/database/migrations"
	"paycore/api/database/seeders"
	"paycore/api/internal/config"
	"paycore/api/internal/database"
)

func main() {
	refresh := flag.Bool("refresh", false, "Reset fixture users and revoke their sessions; leave other users unchanged")
	flag.Parse()
	cfg := config.Load()
	switch strings.ToLower(cfg.Environment) {
	case "development", "local", "test":
	default:
		log.Fatal("Sample user seeding is available only in development, local or test environments.")
	}
	password := os.Getenv("PAYCORE_SEED_PASSWORD")
	if password == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			log.Fatal("Run interactively or set PAYCORE_SEED_PASSWORD. Passwords are never accepted as command arguments.")
		}
		fmt.Print("Development seed password (8 to 72 bytes): ")
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			log.Fatal("Could not read password.")
		}
		fmt.Print("Confirm password: ")
		confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil || string(value) != string(confirmation) {
			clear(value)
			clear(confirmation)
			log.Fatal("Passwords did not match.")
		}
		password = string(value)
		clear(value)
		clear(confirmation)
	}
	if len(password) < 8 || len(password) > 72 {
		log.Fatal("Development seed password must be 8 to 72 bytes.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Database connection failed. Start PostgreSQL and check DATABASE_URL.")
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		log.Fatal("Database migration failed; seeding stopped.")
	}
	if err = seeders.RBAC(ctx, pool); err != nil {
		log.Fatal("RBAC seeding failed; seeding stopped.")
	}
	seed := seeders.Users
	if *refresh {
		seed = seeders.RefreshUsers
	}
	results, err := seed(ctx, pool, password)
	if err != nil {
		log.Fatal("User seeding failed; no partial set of sample users was committed.")
	}
	inserted := 0
	refreshed := 0
	fmt.Printf("%-16s %-16s %-28s %-30s %s\n", "first_name", "last_name", "email", "roles", "status")
	for _, result := range results {
		status := "skipped (unchanged)"
		if result.Refreshed {
			refreshed++
			status = "refreshed"
		} else if result.Inserted {
			inserted++
			status = "created"
		}
		lastName := "NULL"
		if result.LastName != nil {
			lastName = *result.LastName
		}
		fmt.Printf("%-16s %-16s %-28s %-30s %s\n", result.FirstName, lastName, result.Email, strings.Join(result.Roles, ", "), status)
	}
	fmt.Printf("Users seeded: %d created, %d refreshed, %d skipped.\n", inserted, refreshed, len(results)-inserted-refreshed)
}
