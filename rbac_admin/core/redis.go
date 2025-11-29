package core

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"rbac.admin/global"
)

func InitRedis() *redis.Client {
	r := global.Config.Redis
	c := redis.NewClient(&redis.Options{
		Addr:     r.Addr,
		Password: r.Password,
		DB:       r.DB,
	})

	_, err := c.Ping(context.Background()).Result()
	if err != nil {
		logrus.Errorf("连接redis失败 %s", err)
	}
	logrus.Infof("成功连接redis")
	return c
}
