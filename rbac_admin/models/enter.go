package models

import (
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"time"
)

type Model struct {
	ID       uint      `gorm:"primaryKey"`
	CreateAt time.Time `gorm:"autoCreateTime" json:"createAt"`
	UpdateAt time.Time `gorm:"autoUpdateTime" json:"updateAt"`
}

type Page struct {
	Limit int    `json:"limit"`
	Page  int    `json:"page"`
	Sort  string `json:"sort"`
	Key   string `json:"key"`
}

type IDRequest struct {
	ID uint `json:"id" form:"id" uri:"id"`
}

type IDListRequest struct {
	IDList []uint `json:"idList"`
}

// 用户表
type UserModel struct {
	Model
	Username     string      `gorm:"size:16,unique" json:"username"`
	Nickname     string      `gorm:"size:32" json:"nickname"`
	Avatar       string      `gorm:"size:256" json:"avatar"`
	Email        string      `gorm:"size:32" json:"email"`
	Password     string      `gorm:"size:256" json:"-"`
	IsSuperAdmin bool        `gorm:"default:false" json:"isSuperAdmin"`
	RoleList     []RoleModel `gorm:"many2many:user_role_models;joinForeignKey:UserID;JoinReferences:RoleID" json:"roleList"`
}

func (u UserModel) BeforeDelete(tx *gorm.DB) error {
	var userRoleList []UserRoleModel
	err := tx.Find(&userRoleList, "user_id = ?", u.ID).Delete(&userRoleList).Error
	logrus.Infof("删除用户关联的角色 %d 条", len(userRoleList))
	return err
}

func (u *UserModel) GetRoleList() []uint {
	// 如果RoleList为空或nil，直接返回空切片
	if len(u.RoleList) == 0 {
		return []uint{}
	}

	// 预分配容量，提高性能
	roleList := make([]uint, 0, len(u.RoleList))
	for _, model := range u.RoleList {
		roleList = append(roleList, model.ID)
	}
	return roleList
}

// 角色表
type RoleModel struct {
	Model
	Title    string      `gorm:"size:16,unique" json:"title"`
	UserList []UserModel `gorm:"many2many:user_role_models;joinForeignKey:RoleID;JoinReferences:UserID" json:"roleList"`
	MenuList []MenuModel `gorm:"many2many:role_menu_models;joinForeignKey:RoleID;JoinReferences:MenuID" json:"menuList"`
}

// 用户角色表
type UserRoleModel struct {
	Model
	UserID    uint      `json:"userID"`
	UserModel UserModel `gorm:"foreignKey:UserID" json:"-"`
	RoleID    uint      `json:"roleID"`
	RoleModel RoleModel `gorm:"foreignKey:RoleID" json:"-"`
}

// 菜单表
type Meta struct {
	Icon  string `gorm:"size:256" json:"icon"`
	Title string `gorm:"size:32" json:"title"`
}
type MenuModel struct {
	Model
	Enable          bool   `json:"enable"` // 是否显示
	Name            string `gorm:"size:32,unique" json:"name"`
	Path            string `gorm:"size:128" json:"path"`
	Component       string `gorm:"size:128" json:"component"`
	Meta            `gorm:"embedded" json:"meta"`
	ParentMenuID    *uint       `json:"parentMenuID"` // 父菜单id
	ParentMenuModel *MenuModel  `gorm:"foreignKey:ParentMenuID" json:"-"`
	Children        []MenuModel `gorm:"foreignKey:ParentMenuID" json:"children"` // 子菜单
	Sort            int         `json:"sort"`
}

type ApiModel struct {
	Model
	Name   string `gorm:"size:256" json:"name"`
	Path   string `gorm:"size:256" json:"path"`
	Method string `gorm:"size:256" json:"method"`
	Group  string `gorm:"size:256" json:"group"` // api分组
}

type RoleMenuModel struct {
	Model
	RoleID    uint      `json:"roleID"`
	RoleModel RoleModel `gorm:"foreignKey:RoleID" json:"-"`
	MenuID    uint      `json:"menuID"`
	MenuModel MenuModel `gorm:"foreignKey:MenuID" json:"-"`
}
