package config

type Jwt struct {
	Expires int    `yaml:"expires"` // 过期时间 单位小时
	Issuer  string `yaml:"issuer"`  // 签发人
	Secret  string `yaml:"secret"`  // 秘钥
}
