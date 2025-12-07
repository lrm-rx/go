package main

import (
	"context"
	"errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"time"
)

const (
	defaultShelfSize = 100
)

// 使用GORM
func NewDB(dsn string) (*gorm.DB, error) {
	// 使用 MySQL 驱动连接数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
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
	Title    string
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
	v := Shelf{Theme: data.Theme, Size: size}
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

// 根据书架id查询图书
func (b *bookstore) GetBookListByShelfID(ctx context.Context, shelfID int64, cursor string, pageSize int) ([]*Book, error) {
	var vl []*Book
	err := b.db.WithContext(ctx).Where("shelf_id = ? and id > ?", shelfID, cursor).Order("id asc").Limit(pageSize).Find(&vl).Error
	return vl, err
}
