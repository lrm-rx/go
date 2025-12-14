package captcha_api

import (
	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
	"rbac.admin/common/res"
	"rbac.admin/utils/captcha"
)

type CaptchaAPI struct {
}

type GenerateCaptchaResponse struct {
	CaptchaID string `json:"captchaID"`
	Captcha   string `json:"captcha"`
}

func (CaptchaAPI) GenerateCaptchaView(c *gin.Context) {

	var driver = base64Captcha.DriverString{
		Width:           200,
		Height:          60,
		NoiseCount:      2,
		ShowLineOptions: 0,
		Length:          6,
		Source:          "012356789",
	}
	cp := base64Captcha.NewCaptcha(&driver, captcha.CaptchaStore)
	id, b64s, _, err := cp.Generate()
	if err != nil {
		logrus.Errorf("图片验证码生成失败 %s", err)
		res.FailWidthMsg("图片验证码生成失败", c)
		return
	}
	res.OkWidthData(GenerateCaptchaResponse{
		CaptchaID: id,
		Captcha:   b64s,
	}, c)
}
