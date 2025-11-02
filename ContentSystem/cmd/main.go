package main

import (
	"ContentSystem/internal/api"
	"fmt"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	api.CmsRouters(r)
	// Start server on port 8080 (default)
	err := r.Run()
	if err != nil {
		fmt.Printf("r run error = %v", err)
		return
	}
}
