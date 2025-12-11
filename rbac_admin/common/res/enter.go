package res

import "github.com/gin-gonic/gin"

type Response struct {
	Code int64  `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func response(code int64, data any, msg string, c *gin.Context) {
	r := Response{
		Code: code,
		Data: data,
		Msg:  msg,
	}
	c.JSON(200, r)
}

func OkWidthData(data any, c *gin.Context) {
	response(0, data, "成功", c)
}

func OkWidthMsg(msg string, c *gin.Context) {
	response(0, gin.H{}, msg, c)
}

func Ok(data any, msg string, c *gin.Context) {
	response(0, data, msg, c)
}

func Fail(code int64, msg string, c *gin.Context) {
	response(code, gin.H{}, msg, c)
}

func FailWidthMsg(msg string, c *gin.Context) {
	response(1001, gin.H{}, msg, c)
}

func FailWidthError(error error, c *gin.Context) {
	response(1001, gin.H{}, error.Error(), c)
}
