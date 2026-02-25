package email_api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/utils/captcha"
	"rbac.admin/utils/email"
	"rbac.admin/utils/random"
)

type EmailAPI struct {
}

type SendEmailRequest struct {
	Email       string `json:"email" binding:"required,email"`
	CaptchaID   string `json:"captchaID"`
	CaptchaCode string `json:"captchaCode"`
}

type SendEmailResponse struct {
	EmailID string `json:"emailID"`
}

func (EmailAPI) SendEmailView(c *gin.Context) {
	cr, err := middleware.GetBind[SendEmailRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}
	// 验证码长度检查
	if len(cr.CaptchaCode) != 6 {
		res.FailWithMsg("验证码长度必须为6位", c)
		return
	}
	if !global.Config.Email.Verify() {
		res.FailWithMsg("尚未配置邮箱, 无法注册", c)
		return
	}
	if global.Config.Captcha.Enable {
		// 启用了验证码
		if cr.CaptchaID == "" || cr.CaptchaCode == "" {
			res.FailWithMsg("请输入图片验证码", c)
			return
		}
		if !captcha.CaptchaStore.Verify(cr.CaptchaID, cr.CaptchaCode, true) {
			res.FailWithMsg("图片验证码验证失败", c)
			return
		}
	}
	emailID := uuid.New().String()
	code := random.RandStrByCode("012356789", 6)
	email.Set(emailID, cr.Email, code)
	/**
	var driver = base64Captcha.DriverString{
		Width:           200,
		Height:          60,
		NoiseCount:      2,
		ShowLineOptions: 0,
		Length:          6,
		Source:          "012356789",
	}
	// todo
	cp := base64Captcha.NewCaptcha(&driver, captcha.CaptchaStore)
	id, _, code, err := cp.Generate()
	if err != nil {
		logrus.Errorf("图片验证码生成失败 %s", err)
		res.FailWithMsg("图片验证码生成失败", c)
		return
	}
	*/

	content := fmt.Sprintf("用户注册的验证码为 %s 5分钟内有效!", code)
	err = email.SendEmail("用户注册", content, cr.Email)
	if err != nil {
		logrus.Errorf("邮件发送失败 %s", err)
		res.FailWithMsg("邮件发送失败", c)
		return
	}
	res.OkWithData(SendEmailResponse{
		EmailID: emailID,
	}, c)
}
