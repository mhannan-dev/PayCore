package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"

	"paycore/api/database/migrations"
	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"

	"golang.org/x/term"
)

func main() {
	emailFlag := flag.String("email", "", "User email")
	firstNameFlag := flag.String("first-name", "", "Required first name")
	lastNameFlag := flag.String("last-name", "", "Optional last name; omitted means NULL")
	var rolesInput string
	flag.StringVar(&rolesInput, "roles", "ACCOUNTANT", "Comma-separated roles, e.g. ADMIN,ACCOUNTANT")
	flag.StringVar(&rolesInput, "role", "ACCOUNTANT", "Alias for -roles")
	flag.Parse()
	email, firstName := strings.ToLower(strings.TrimSpace(*emailFlag)), strings.TrimSpace(*firstNameFlag)
	lastNameValue := strings.TrimSpace(*lastNameFlag)
	var lastName *string
	if lastNameValue != "" {
		lastName = &lastNameValue
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || len(firstName) == 0 || len([]rune(firstName)) > 80 || len([]rune(lastNameValue)) > 80 {
		log.Fatal("Provide a valid -email, a -first-name of 1 to 80 characters, and an optional -last-name of at most 80 characters.")
	}
	roles, err := models.NormalizeRoles(strings.Split(rolesInput, ","))
	if err != nil {
		log.Fatal("Provide at least one valid role using -roles.")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		log.Fatal("Run interactively: passwords are read from the terminal without echo.")
	}
	fmt.Print("Password (exactly 10 characters): ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		log.Fatal("Could not read password.")
	}
	fmt.Print("Confirm password: ")
	confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil || string(password) != string(confirmation) {
		log.Fatal("Passwords did not match.")
	}
	hash, err := services.HashPassword(string(password))
	clear(password)
	clear(confirmation)
	if err != nil {
		log.Fatal("Password must be exactly 10 characters.")
	}
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Database connection failed.")
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		log.Fatal("Database migration failed.")
	}
	u, err := repositories.NewUserRepository(pool).Create(ctx, firstName, lastName, email, hash, roles)
	if err != nil {
		log.Fatal("User creation failed. Verify the email is not already registered.")
	}
	fmt.Printf("User created: %s (%s), roles %s\n", u.ID, u.Email, strings.Join(u.Roles, ", "))
}
