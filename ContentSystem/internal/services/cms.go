package services

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type CmsApp struct {
	db *gorm.DB
}

func NewCmsApp() *CmsApp {
	app := &CmsApp{}
	connDB(app)
	return app
}

func connDB(app *CmsApp) {
	// 数据库名称已在model中指定, 这里不配置 root:756131502@tcp(localhost:3006)/cms_account?charset=utf8mb4&parseTime=True&loc=Local
	mysqlDB, err := gorm.Open(mysql.Open("root:756131502@tcp(localhost:3006)/?charset=utf8mb4&parseTime=True&loc=Local"))
	if err != nil {
		panic(err)
	}
	db, err := mysqlDB.DB()
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	app.db = mysqlDB
}
