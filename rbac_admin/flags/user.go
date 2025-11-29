package flags

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"rbac.admin/global"
	"rbac.admin/models"
	pwd "rbac.admin/utils"
)

type User struct {
}

func (User) CreateSuperAdmin() {
	fmt.Println("请输入用户名")
	var username string
	fmt.Scanln(&username)
	var user models.UserModel
	err := global.DB.Take(&user, "username = ?", username).Error
	if err == nil {
		logrus.Errorf("用户名已经存在 %s", err)
		return
	}
	fmt.Println("请输入密码")
	password, err := terminal.ReadPassword(int(os.Stdin.Fd())) // 读取用户输入的密码
	if err != nil {
		logrus.Errorf("读取密码时出错 %s", err)
		return
	}
	fmt.Println("请两次输入密码")
	rePassword, err := terminal.ReadPassword(int(os.Stdin.Fd())) // 读取用户输入的密码
	if err != nil {
		logrus.Errorf("读取密码时出错 %s", err)
		return
	}
	if string(password) != string(rePassword) {
		logrus.Errorf("两次密码不一致")
		return
	}
	hashPwd := pwd.HashPassword(string(password))
	err = global.DB.Create(&models.UserModel{
		Username:     username,
		Password:     hashPwd,
		IsSuperAdmin: true,
	}).Error
	if err != nil {
		logrus.Errorf("创建用户失败 %s", err)
		return
	}
	logrus.Infof("创建用户成功")
}
