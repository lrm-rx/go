package dao

import (
	"ContentSystem/internal/model"
	"gorm.io/gorm"
	"log"
)

type ContentDao struct {
	db *gorm.DB
}

func NewContentDao(db *gorm.DB) *ContentDao {
	return &ContentDao{db: db}
}

func (c *ContentDao) Create(detail model.ContentDetail) (int, error) {
	if err := c.db.Create(&detail).Error; err != nil {
		log.Printf("content create error = %v", err)
		return 0, err
	}
	return detail.ID, nil
}
