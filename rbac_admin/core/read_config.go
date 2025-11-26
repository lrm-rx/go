package core

import (
	"gopkg.in/yaml.v3"
	"os"
	"rbac.admin/config"
)

func ReadConfig() *config.Config {
	byteData, err := os.ReadFile("settings.yaml")
	if err != nil {
		panic("配置文件读取失败" + err.Error())
		return nil
	}
	var c *config.Config
	err = yaml.Unmarshal(byteData, &c)
	if err != nil {
		panic("配置文件格式错误" + err.Error())
		return nil
	}
	return c
}
