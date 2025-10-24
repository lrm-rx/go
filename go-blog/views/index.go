package views

import (
	"go-blog/common"
	"go-blog/config"
	"go-blog/models"
	"net/http"
)

func (*HTMLApi) Index(w http.ResponseWriter, r *http.Request) {
	index := common.Template.Index
	// 页面上涉及到的所有数据都必须定义
	categorys := []models.Category{
		{
			Cid:  1,
			Name: "go",
		},
	}
	posts := []models.PostMore{
		{
			Pid:          1,
			Title:        "go博客",
			Content:      "内容",
			UserName:     "张三",
			ViewCount:    123,
			CreateAt:     "2022-02-20",
			CategoryId:   1,
			CategoryName: "go",
			Type:         0,
		},
	}
	hr := &models.HomeResponse{
		config.Cfg.Viewer,
		categorys,
		posts,
		1,
		1,
		[]int{1},
		true,
	}
	index.WriteData(w, hr)
}
