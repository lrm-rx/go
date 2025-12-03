package main

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"os"
	"time"
)

const (
	defaultShelfSize = 100
)

// 使用GORM
func NewDB(dsn string) (*gorm.DB, error) {
	dbFile := "test.db"

	// 检查文件是否存在
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		// 文件不存在，创建空文件
		file, err := os.Create(dbFile)
		if err != nil {
			panic("创建数据库文件失败: " + err.Error())
		}
		file.Close()
		fmt.Println("数据库文件已创建:", dbFile)
	}
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		panic("连接数据库失败: " + err.Error())
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

// 数据库操作
type bookstore struct {
	db *gorm.DB
}

// 创建书架
func (b *bookstore) CreateShelf(ctx context.Context, data Shelf) (*Shelf, error) {
	if len(data.Theme) <= 0 {
		return nil, errors.New("invalid theme")
	}
	size := data.Size
	if size <= 0 {
		size = defaultShelfSize
	}
	v := Shelf{Theme: data.Theme, Size: data.Size}
	err := b.db.WithContext(ctx).Create(&v).Error
	return &v, err
}

// 获取书架
func (b *bookstore) GetShelf(ctx context.Context, id int64) (*Shelf, error) {
	v := Shelf{}
	err := b.db.WithContext(ctx).First(&v, id).Error
	return &v, err
}

// 书架列表
func (b *bookstore) ListShelf(ctx context.Context) ([]*Shelf, error) {
	var vl []*Shelf
	err := b.db.WithContext(ctx).Find(&vl).Error
	return vl, err
}

// 删除书架
func (b *bookstore) DeleteShelf(ctx context.Context, id int64) error {
	return b.db.WithContext(ctx).Delete(&Shelf{}, id).Error
}
