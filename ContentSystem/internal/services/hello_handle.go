package services

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

type HelloReq struct {
	Name string `json:"name" binding:"required"`
}

type HelloRsp struct {
	Message string `json:"message" binding:"required"`
}

func (c *CmsApp) Hello(ctx *gin.Context) {
	var req HelloReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Return JSON response
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ok",
		"data": &HelloRsp{
			Message: fmt.Sprintf("hello %s", req.Name),
		},
	})
}
