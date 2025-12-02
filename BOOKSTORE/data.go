package main

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"time"
)

// 使用GORM

func NewDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		panic("连接数据库失败!")
	}

	// 迁移 schema
	db.AutoMigrate(&Shelf{}, &Book{})
	return db, nil
}

// 书架
type Shelf struct {
	ID       int64 `gorm:"primaryKey"`
	Theme    string
	Size     int64
	CreateAt time.Time `gorm:"autoCreateTime"`
	UpdateAt time.Time `gorm:"autoUpdateTime"`
}

type Book struct {
	ID       int64 `gorm:"primaryKey"`
	Author   string
	title    string
	ShelfID  int64
	CreateAt time.Time `gorm:"autoCreateTime"`
	UpdateAt time.Time `gorm:"autoUpdateTime"`
}
