package core

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"rbac.admin/config"
)

func SetConfig(c *config.Config) {
	byteData, _ := yaml.Marshal(c)
	err := os.WriteFile("settings.yaml", byteData, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}
}
