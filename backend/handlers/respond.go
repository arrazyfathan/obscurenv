package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func errorJSON(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func badRequest(c *gin.Context, message string) {
	errorJSON(c, http.StatusBadRequest, message)
}
