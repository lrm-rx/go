package main

import (
	"go-blog/config"
	"go-blog/models"
	"html/template"
	"log"
	"net/http"
	"time"
)

func isODD(num int) bool {
	return num%2 == 0
}
func GetNextName(strs []string, index int) string {
	return strs[index+1]
}
func Date(layout string) string {
	return time.Now().Format(layout)
}
func index(w http.ResponseWriter, r *http.Request) {
	t := template.New("index.html")
	// 1. 拿当前路径
	path := config.Cfg.System.CurrentDir
	// 访问博客首页模板的时候, 因为有多个模板的嵌套, 解析文件的时候, 需要将其涉及到的所胡模板都进行解析
	index := path + "/template/index.html"
	home := path + "/template/home.html"
	header := path + "/template/layout/header.html"
	footer := path + "/template/layout/footer.html"
	personal := path + "/template/layout/personal.html"
	post := path + "/template/layout/post-list.html"
	pagination := path + "/template/layout/pagination.html"
	t.Funcs(template.FuncMap{"isODD": isODD, "getNextName": GetNextName, "date": Date})
	t, err := t.ParseFiles(index, home, header, footer, personal, post, pagination)
	if err != nil {
		log.Println("解析模板错误:", err)
	}
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
	t.Execute(w, hr)
}
func main() {
	// 程序入口
	server := http.Server{
		Addr: "127.0.0.1:8080",
	}
	// 路由
	http.HandleFunc("/", index)
	// 静态资源配置
	http.Handle("/resource/", http.StripPrefix("/resource/", http.FileServer(http.Dir("public/resource/"))))
	if err := server.ListenAndServe(); err != nil {
		log.Println(err)
	}
}
