package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

type ProjectHandler struct {
	db *sql.DB
}

func NewProjectHandler(database *sql.DB) *ProjectHandler {
	return &ProjectHandler{db: database}
}

type createProjectRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

func (h *ProjectHandler) Create(c *gin.Context) {
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid project request")
		return
	}
	var id string
	err := h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO projects (user_id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id
	`, middleware.UserID(c), req.Name, req.Slug).Scan(&id)
	if err != nil {
		errorJSON(c, http.StatusConflict, "project already exists")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "slug": req.Slug})
}
