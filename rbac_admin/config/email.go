package config

type Email struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// 校验邮箱是否配置
func (e Email) Verify() bool {
	if e.User == "" || e.Password == "" || e.Host == "" || e.Port == 0 {
		return false
	}
	return true
}
