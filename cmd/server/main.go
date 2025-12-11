package main

import (
	"log"
	"net/http"
	"fmt"

	"trackr/internal/api"
	"trackr/internal/api/handlers"
	"trackr/internal/api/middleware"
	"trackr/internal/platform/auth"
	"trackr/internal/platform/config"
	"trackr/internal/platform/database"
	"trackr/internal/platform/repositories"
	"trackr/internal/pkg/logger"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Init(cfg.Logging)

	// Database Connections
	globalDB, err := database.NewGlobalDB(cfg.Database.Global)
	if err != nil {
		log.Fatalf("Failed to connect to global DB: %v", err)
	}
	defer globalDB.Close()

	tenantDBPool := database.NewTenantDBPool(cfg.Database.Tenant)
	defer tenantDBPool.CloseAll()

	// Repositories
	orgRepo := repositories.NewOrganizationRepository(globalDB)
	userRepo := repositories.NewUserRepository(globalDB)
	inviteRepo := repositories.NewInviteRepository(globalDB)

	// Services
	tokenSvc := auth.NewTokenService(cfg.JWT)

	// Handlers
	authHandler := handlers.NewAuthHandler(userRepo, orgRepo, inviteRepo, tokenSvc)
	orgHandler := handlers.NewOrgHandler(orgRepo, userRepo, tokenSvc)
	inviteHandler := handlers.NewInviteHandler(inviteRepo)
	userHandler := handlers.NewUserHandler()

	// Middleware
	authMiddleware := middleware.NewAuthMiddleware(tokenSvc)
	tenantMiddleware := middleware.NewTenantMiddleware(orgRepo, tenantDBPool)

	// Router
	deps := &api.Dependencies{
		AuthHandler:      authHandler,
		OrgHandler:       orgHandler,
		InviteHandler:    inviteHandler,
		UserHandler:      userHandler,
		AuthMiddleware:   authMiddleware,
		TenantMiddleware: tenantMiddleware,
	}
	router := api.NewRouter(deps)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Server starting on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
