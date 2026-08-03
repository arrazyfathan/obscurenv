package main

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/db"
	"github.com/obscurenv/obscurenv/backend/handlers"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

func main() {
	databaseURL := getenv("DATABASE_URL", "postgres://obv:obv@localhost:5432/obv?sslmode=disable")
	addr := listenAddr()

	database, err := db.Open(databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	router := gin.Default()
	registerHealthRoutes(router)
	api := router.Group("/api/v1")
	authHandler := handlers.NewAuthHandler(database)
	api.POST("/auth/register", authHandler.Register)
	api.POST("/auth/login", authHandler.Login)
	api.POST("/auth/passkey/login/options", authHandler.PasskeyLoginOptions)
	api.POST("/auth/passkey/login/finish", authHandler.PasskeyLoginFinish)

	protected := api.Group("/")
	protected.Use(middleware.Auth(database))
	projectHandler := handlers.NewProjectHandler(database)
	envHandler := handlers.NewEnvHandler(database)
	userHandler := handlers.NewUserHandler(database)
	protected.GET("/auth/passkeys", authHandler.ListPasskeys)
	protected.DELETE("/auth/passkeys/:id", authHandler.RevokePasskey)
	protected.POST("/auth/passkey/register/options", authHandler.PasskeyRegisterOptions)
	protected.POST("/auth/passkey/register/finish", authHandler.PasskeyRegisterFinish)
	protected.GET("/user/profile", userHandler.Profile)
	protected.PATCH("/user/profile", userHandler.UpdateProfile)
	protected.GET("/projects", projectHandler.List)
	protected.GET("/projects/:slug", projectHandler.Get)
	protected.POST("/projects", projectHandler.Create)
	protected.POST("/env/push", envHandler.Push)
	protected.GET("/env/pull", envHandler.Pull)
	protected.GET("/env/list", envHandler.List)
	protected.GET("/env/versions", envHandler.Versions)

	if err := router.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func registerHealthRoutes(router *gin.Engine) {
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"name": "obscurenv", "status": "ok"})
	})
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func listenAddr() string {
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr
	}
	port := getenv("PORT", "8080")
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}
