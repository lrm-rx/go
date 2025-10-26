package views

import (
	"errors"
	"go-blog/common"
	"go-blog/service"
	"log"
	"net/http"
	"strconv"
	"strings"
)

func (*HTMLApi) Category(w http.ResponseWriter, r *http.Request) {
	categoryTemplate := common.Template.Category
	path := r.URL.Path
	cIdStr := strings.TrimPrefix(path, "/c/")
	cId, err := strconv.Atoi(cIdStr)
	if err != nil {
		categoryTemplate.WriteError(w, errors.New("请求路径不匹配!"))
		return
	}
	if err := r.ParseForm(); err != nil {
		log.Println("表单数据获取失败:", err)
		categoryTemplate.WriteError(w, errors.New("系统出错, 请联系管理员!"))
		return
	}
	pageStr := r.Form.Get("page")
	if pageStr == "" {
		pageStr = "1"
	}
	page, _ := strconv.Atoi(pageStr)
	pageSize := 10
	categoryResponse, err := service.GetPostsByCategoryId(cId, page, pageSize)
	if err != nil {
		log.Println("Category获取数据出错:", err)
		categoryTemplate.WriteError(w, errors.New("系统出错, 请联系管理员!"))
	}
	categoryTemplate.WriteData(w, categoryResponse)
}
