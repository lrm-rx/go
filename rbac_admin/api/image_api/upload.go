package image_api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/middleware"
)

func (ImageAPI) UploadView(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		res.FailWidthMsg("获取文件失败", c)
		return
	}

	auth := middleware.GetAuth(c)
	dst := fmt.Sprintf("uploads/images/%s/%s", auth.Username, fileHeader.Filename)
	err = c.SaveUploadedFile(fileHeader, dst)
	if err != nil {
		res.FailWidthMsg("保存文件失败", c)
		return
	}
	res.Ok("/"+dst, "图片上传成功", c)
}
