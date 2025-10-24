package router

import (
	"go-blog/api"
	"go-blog/views"
	"net/http"
)

func Router() {
	// 1. 页面 2.数据(json) 3.静态资源
	http.HandleFunc("/", views.HTML.Index)
	http.HandleFunc("/api/v1/post", api.API.SaveAndUpdatePost)
	// 静态资源配置
	http.Handle("/resource/", http.StripPrefix("/resource/", http.FileServer(http.Dir("public/resource/"))))

}
