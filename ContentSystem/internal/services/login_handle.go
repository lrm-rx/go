package services

import (
	"ContentSystem/internal/dao"
	"ContentSystem/internal/utils"
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

type LoginReq struct {
	UserID   string `json:"userID" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginRsp struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Nickname  string `json:"nickname"`
}

func (c *CmsApp) Login(ctx *gin.Context) {
	var req LoginReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var (
		userID   = req.UserID
		password = req.Password
	)
	accountDao := dao.NewAccountDao(c.db)
	account, err := accountDao.FirstByUserID(ctx.Request.Context(), userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请输入正确的账号ID"})
		return
	}
	if err := bcrypt.CompareHashAndPassword(
		[]byte(account.Password),
		[]byte(password)); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请输入正确的密码"})
		return
	}
	sessionID, err := c.generateSessionID(ctx.Request.Context(), userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "系统错误，请稍后重试"})
		return
	}
	// 回包
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ok",
		"data": &LoginRsp{
			SessionID: sessionID,
			UserID:    account.UserID,
			Nickname:  account.Nickname,
		},
	})
}

func (c *CmsApp) generateSessionID(ctx context.Context, userID string) (string, error) {
	sessionID := uuid.New().String()
	// key : session_id:{user_id} val : session_id  20s
	sessionKey := utils.GetSessionKey(userID)
	err := c.rdb.Set(ctx, sessionKey, sessionID, time.Hour*8).Err()
	if err != nil {
		fmt.Printf("rdb set error = %v \n", err)
		return "", err
	}
	authKey := utils.GetAuthKey(sessionID)
	err = c.rdb.Set(ctx, authKey, time.Now().Unix(), time.Hour*8).Err()
	if err != nil {
		fmt.Printf("rdb set error = %v \n", err)
		return "", err
	}
	return sessionID, nil
}
