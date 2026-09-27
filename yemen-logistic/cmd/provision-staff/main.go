// Command provision-staff creates a single internal staff account (admin or
// officer) with a bcrypt-hashed password. Meant to be run once, by hand, by
// an operator with database access never scripted with a hardcoded
// password, and never folded into a schema migration (a committed hash is
// a standing offline-crackable secret in git history forever).
//
// Usage:

//
// The password is never accepted as a flag (it would leak into shell
// history and process listings); it is read from a hidden terminal prompt,
// twice, and must match on both entries.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/domain"
	"yemen-mit-platform/internal/repository/postgres"
)

func main() {
	username := flag.String("username", "", "login username for the new staff account (required)")
	role := flag.String("role", "", "staff role: admin or officer (required)")
	nameAr := flag.String("name-ar", "", "full name, Arabic (required)")
	nameEn := flag.String("name-en", "", "full name, English (required)")
	titleAr := flag.String("title-ar", "", "job title, Arabic (required)")
	titleEn := flag.String("title-en", "", "job title, English (required)")
	flag.Parse()

	if *username == "" || *nameAr == "" || *nameEn == "" || *titleAr == "" || *titleEn == "" {
		fmt.Fprintln(os.Stderr, "error: -username, -name-ar, -name-en, -title-ar, -title-en are all required")
		os.Exit(1)
	}

	staffRole := domain.StaffRole(*role)
	if !staffRole.IsValid() {
		fmt.Fprintf(os.Stderr, "error: -role must be %q or %q, got %q\n", domain.StaffRoleAdmin, domain.StaffRoleOfficer, *role)
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "error: DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		fmt.Fprintln(os.Stderr, "error: JWT_SECRET environment variable is required")
		os.Exit(1)
	}
	// tokenTTL is irrelevant here -- this command never issues a token, it
	// only needs auth.Service for its bcrypt HashPassword helper.
	authSvc, err := auth.NewService(jwtSecret, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprint(os.Stderr, "Enter password for new staff account: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading password: %v\n", err)
		os.Exit(1)
	}
	if len(passwordBytes) < 12 {
		fmt.Fprintln(os.Stderr, "error: password must be at least 12 characters for a government staff account")
		os.Exit(1)
	}

	fmt.Fprint(os.Stderr, "Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading password confirmation: %v\n", err)
		os.Exit(1)
	}
	if string(passwordBytes) != string(confirmBytes) {
		fmt.Fprintln(os.Stderr, "error: passwords do not match")
		os.Exit(1)
	}

	hash, err := authSvc.HashPassword(string(passwordBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error hashing password: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	staffRepo := postgres.NewStaffRepository(pool)

	created, err := staffRepo.Create(ctx, domain.Staff{
		Username:     *username,
		PasswordHash: hash,
		Role:         staffRole,
		NameAr:       *nameAr,
		NameEn:       *nameEn,
		TitleAr:      *titleAr,
		TitleEn:      *titleEn,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating staff account: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created staff account: id=%s username=%s role=%s\n", created.ID, created.Username, created.Role)
}