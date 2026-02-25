package query

import (
	"fmt"
	"gorm.io/gorm"
	"rbac.admin/global"
	"rbac.admin/models"
)

type Option struct {
	Page     models.Page
	Likes    []string
	Where    *gorm.DB
	Debug    bool
	Preloads []string
}

func List[T any](model T, option Option) (list []T, count int64, err error) {
	// 带入model
	baseDB := global.DB.Model(model).Where(model)
	// 带入debug
	if option.Debug {
		baseDB = baseDB.Debug()
	}
	// 带入where
	if option.Where != nil {
		baseDB = baseDB.Where(option.Where)
	}
	// 带入likes
	if option.Page.Key != "" && len(option.Likes) > 0 {
		query := global.DB.Where("")
		for _, column := range option.Likes {
			query.Or(fmt.Sprintf("%s like ?", column), fmt.Sprintf("%%%s%%", option.Page.Key))
		}
		baseDB = baseDB.Where(query)
	}

	// 加载preloads
	for _, preload := range option.Preloads {
		baseDB = baseDB.Preload(preload)
	}

	if option.Page.Limit <= 0 {
		option.Page.Limit = 10
	}
	if option.Page.Page <= 0 {
		option.Page.Page = 1
	}
	// 带入分页
	offset := (option.Page.Page - 1) * option.Page.Limit
	if option.Page.Sort == "" {
		option.Page.Sort = "create_at desc"
	}

	baseDB.Offset(offset).Limit(option.Page.Limit).Order(option.Page.Sort).Find(&list)
	baseDB.Model(model).Count(&count)
	return
}
