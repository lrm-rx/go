package service

import (
	"errors"
	"go-blog/models"
	"go-blog/utils"
	"log"
)

func Login(userName, passwd string) (*models.LoginRes, error) {
	passwd = utils.Md5Crypt(passwd, "asdfghjkl")

	//user := dao.GetUser(userName, passwd)
	//if user == nil {
	//	return nil, errors.New("账号或密码不正确!")
	//}
	//uid := user.Uid
	// 生成token
	var uid = 1
	token, err := utils.Award(&uid)
	log.Println(token)
	if err != nil {
		return nil, errors.New("无法生成token!")
	}
	var userInfo models.UserInfo
	//userInfo.Uid = user.Uid
	//userInfo.UserName = user.UserName
	//userInfo.Avatar = user.Avatar
	userInfo.Uid = 1
	userInfo.UserName = "admin"
	userInfo.Avatar = ""
	var lr = &models.LoginRes{
		token,
		userInfo,
	}
	return lr, nil
}
