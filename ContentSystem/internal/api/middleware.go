package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

const SessionKey = "session_id"

type SessionAuth struct {
}

func (s *SessionAuth) Auth(ctx *gin.Context) {
	sessionID := ctx.GetHeader(SessionKey)
	// todo
	if sessionID == "" {
		ctx.AbortWithStatusJSON(http.StatusForbidden, "sessionID is null")
	}
	fmt.Println("session id ", sessionID)
	ctx.Next()
}
