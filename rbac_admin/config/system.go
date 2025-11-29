package config

import "fmt"

type System struct {
	Ip   string `yaml:"ip"`
	Mode string `yaml:"mode"`
	Port int    `yaml:"port"`
}

func (s System) Addr() string {
	return fmt.Sprintf("%s:%d", s.Ip, s.Port)
}
