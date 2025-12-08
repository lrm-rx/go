package user_api

import "github.com/gin-gonic/gin"

func (UserAPI) LoginView(c *gin.Context) {
	c.String(200, "login")
}
