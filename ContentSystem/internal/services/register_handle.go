package services

import (
	"ContentSystem/internal/dao"
	"ContentSystem/internal/model"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

type RegisterReq struct {
	UserID   string `json:"userID" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname" binding:"required"`
}

type RegisterRsp struct {
	Message string `json:"message" binding:"required"`
}

func (c *CmsApp) Register(ctx *gin.Context) {
	var req RegisterReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 账号校验（账号是否存在）
	accountDao := dao.NewAccountDao(c.db)
	isExist, err := accountDao.IsExist(req.UserID)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if isExist {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "账号已存在"})
		return
	}
	// 密码加密
	hashedPassword, err := encryptPassword(req.Password)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{})
	}
	// 账号信息持久化
	nowTime := time.Now()
	if err := accountDao.Create(model.Account{
		UserID:   req.UserID,
		Password: hashedPassword,
		Nickname: req.Nickname,
		Ct:       nowTime,
		Ut:       nowTime,
	}); err != nil {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	fmt.Printf("register req = %+v , hashedPassword = [%s]\n", req, hashedPassword)
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ok",
		"data": &RegisterRsp{
			Message: fmt.Sprintf("注册成功"),
		},
	})
}

func encryptPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("bctypt generate from password error = %v", err)
		return "", err
	}
	return string(hashedPassword), nil
}
