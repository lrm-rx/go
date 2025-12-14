package config

type Config struct {
	System  System  `yaml:"system"`
	DB      DB      `yaml:"db"`
	Redis   Redis   `yaml:"redis"`
	Jwt     Jwt     `yaml:"jwt"`
	Captcha Captcha `yaml:"captcha"`
}
