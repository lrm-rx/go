package core

import (
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
	"rbac.admin/config"
)

func ReadConfig() *config.Config {
	byteData, err := os.ReadFile("settings.yaml")
	if err != nil {
		//panic("配置文件读取失败" + err.Error())
		//return nil
		logrus.Fatalf("配置文件读取失败 %s", err)
	}
	var c *config.Config
	err = yaml.Unmarshal(byteData, &c)
	if err != nil {
		//panic("配置文件格式错误" + err.Error())
		//return nil
		logrus.Fatalf("配置文件格式错误 %s", err)
	}
	return c
}
