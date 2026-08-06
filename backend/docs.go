package main

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed api-docs/*
var apiDocs embed.FS

func registerDocsRoutes(router *gin.Engine) {
	sub, err := fs.Sub(apiDocs, "api-docs")
	if err != nil {
		return
	}
	router.StaticFS("/docs", http.FS(sub))
}
