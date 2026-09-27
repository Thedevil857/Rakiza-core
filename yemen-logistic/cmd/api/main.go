package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"yemen-mit-platform/internal/auth"
	"yemen-mit-platform/internal/checkpoint"
	"yemen-mit-platform/internal/http/handlers"
	"yemen-mit-platform/internal/repository/postgres"
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required, e.g. postgres://user:pass@localhost:5432/sovereign_customs?sslmode=disable")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required (>=32 random bytes) -- generate one with `openssl rand -base64 32`; never hardcode or commit it")
	}

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	authSvc, err := auth.NewService(jwtSecret, 24*time.Hour)
	if err != nil {
		log.Fatalf("auth service init failed: %v", err)
	}

	
	qrPrivateKeyHex := os.Getenv("QR_PRIVATE_KEY_HEX")
	qrPublicKeyHex := os.Getenv("QR_PUBLIC_KEY_HEX")
	if qrPrivateKeyHex == "" || qrPublicKeyHex == "" {
		log.Println("WARNING: QR_PRIVATE_KEY_HEX/QR_PUBLIC_KEY_HEX not set -- generating an EPHEMERAL RSA keypair for THIS RUN ONLY.")
		log.Println("WARNING: every permit QR code issued now will fail to verify after this server restarts, and any officer phone's")
		log.Println("WARNING: cached public key will go stale. Set both env vars to a real, persistent keypair before real deployment.")
		generatedPrivate, generatedPublic, keyErr := checkpoint.GenerateRSAKeyPairHex()
		if keyErr != nil {
			log.Fatalf("failed to bootstrap ephemeral QR keypair: %v", keyErr)
		}
		qrPrivateKeyHex = generatedPrivate
		qrPublicKeyHex = generatedPublic
	}

	merchantRepo := postgres.NewMerchantRepository(pool)
	permitRepo := postgres.NewPermitRepository(pool)
	staffRepo := postgres.NewStaffRepository(pool)
	clearanceRepo := postgres.NewPermitClearanceRepository(pool) 
	authHandler := handlers.NewAuthHandler(merchantRepo, staffRepo, authSvc)
	// (fixed: 3rd argument is the qrPrivateKeyHex bootstrapped above, not
	// the missing 3rd arg that would otherwise be a compile error now that
	// GetQR actually signs the token)
	permitHandler := handlers.NewPermitHandler(permitRepo, merchantRepo, qrPrivateKeyHex)

	onboardingHandler := handlers.NewMerchantOnboardingHandler(merchantRepo)
	reviewHandler := handlers.NewMerchantReviewHandler(merchantRepo)
	checkpointHandler := handlers.NewCheckpointHandler(clearanceRepo, qrPublicKeyHex)

	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	
	allowedOrigins := []string{"http://localhost:5173"}
	if extra := os.Getenv("CORS_ALLOWED_ORIGINS"); extra != "" {
		for _, origin := range strings.Split(extra, ",") {
			trimmed := strings.TrimSpace(origin)
			if trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Merchant-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","time":"` + time.Now().Format(time.RFC3339) + `"}`))
	})

	r.Route("/api", func(api chi.Router) {
		api.Post("/auth/register", authHandler.Register)
		api.Post("/auth/login", authHandler.Login)

		api.Group(func(protected chi.Router) {
			protected.Use(authSvc.Middleware)
			protected.Post("/permits", permitHandler.Create)

			protected.Post("/merchant/onboarding", onboardingHandler.Complete)
			protected.Post("/merchant/me", onboardingHandler.Me)

			protected.Get("/admin/merchants/pending", reviewHandler.ListPending)
			protected.Patch("/admin/merchants/{id}/approve", reviewHandler.Approve)

		
			protected.Get("/checkpoint/public-key", checkpointHandler.PublicKey)
			protected.Patch("/checkpoint/clearances", checkpointHandler.SubmitClearances)
		})

		
		api.Get("/permits/{id}/qr", permitHandler.GetQR)
	})

	log.Println("Sovereign MIT Yemen Server running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
