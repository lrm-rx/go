package image_api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"os"
	"path"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/utils/md5"
	"rbac.admin/utils/random"
	"strings"
)

var WhiteMap = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

func (ImageAPI) UploadView(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		res.FailWidthMsg("获取文件失败", c)
		return
	}
	// 文件大小验证
	if fileHeader.Size > global.Config.Upload.Size*1024*1024 {
		res.FailWidthMsg(fmt.Sprintf("文件大小不能超过%dMB", global.Config.Upload.Size), c)
		return
	}

	// 图片格式验证
	_, ok := WhiteMap[strings.ToLower(path.Ext(fileHeader.Filename))]
	if !ok {
		res.FailWidthMsg("图片格式必须为jpg、jpeg、png、webp、gif", c)
		return
	}

	auth := middleware.GetAuth(c)
	dst := path.Join("uploads", global.Config.Upload.Dir, auth.Username, fileHeader.Filename)
	for {
		// 检查文件是否存在
		if _, err = os.Stat(dst); err != nil {
			// 文件不存在，直接上传
			break
		}
		// 上传的文件和之前的文件重名
		file, _ := fileHeader.Open()
		fileHash := md5.FileToMd5(file)
		oldFile, _ := os.Open(dst)
		oldFileHash := md5.FileToMd5(oldFile)
		if fileHash == oldFileHash {
			// 文件内容一致
			break
		}
		// 文件内容不一致，需要重新命名
		name := strings.TrimRight(fileHeader.Filename, path.Ext(fileHeader.Filename))
		rname := random.RandStr(3)
		ext := path.Ext(fileHeader.Filename)
		dst = fmt.Sprintf("uploads/images/%s/%s_%s%s", auth.Username, name, rname, ext)
	}

	err = c.SaveUploadedFile(fileHeader, dst)
	if err != nil {
		res.FailWidthMsg("保存文件失败", c)
		return
	}
	res.Ok("/"+dst, "图片上传成功", c)
}
