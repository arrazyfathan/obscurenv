package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/db"
	"github.com/obscurenv/obscurenv/backend/handlers"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

func main() {
	databaseURL := getenv("DATABASE_URL", "postgres://obv:obv@localhost:5432/obv?sslmode=disable")
	addr := getenv("ADDR", ":8080")

	database, err := db.Open(databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	router := gin.Default()
	api := router.Group("/api/v1")
	authHandler := handlers.NewAuthHandler(database)
	api.POST("/auth/register", authHandler.Register)
	api.POST("/auth/login", authHandler.Login)

	protected := api.Group("/")
	protected.Use(middleware.Auth(database))
	projectHandler := handlers.NewProjectHandler(database)
	envHandler := handlers.NewEnvHandler(database)
	protected.POST("/projects", projectHandler.Create)
	protected.POST("/env/push", envHandler.Push)
	protected.GET("/env/pull", envHandler.Pull)
	protected.GET("/env/list", envHandler.List)

	if err := router.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
