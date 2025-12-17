package email

import (
	"crypto/tls"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
	"rbac.admin/global"
)

func SendEmail(subject string, content string, users ...string) error {
	e := global.Config.Email
	mailer := gomail.NewMessage()
	mailer.SetHeader("From", e.User)
	mailer.SetHeader("To", users...)
	mailer.SetHeader("Subject", subject) // 主题
	mailer.SetBody("text/html", content) // 正文

	// 构建SMTP客户端
	dialer := gomail.NewDialer(e.Host, e.Port, e.User, e.Password)
	dialer.TLSConfig = &tls.Config{InsecureSkipVerify: true} // 忽略证书校验, 仅用于测试环境
	// 发送邮件
	if err := dialer.DialAndSend(mailer); err != nil {
		logrus.Errorf("发送邮件失败 %s", err)
		return err
	}
	return nil
}
