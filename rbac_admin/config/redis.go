package config

type Redis struct {
	Addr     string `yaml:"addr"`
	DB       int    `yaml:"db"`
	Password string `yaml:"password"`
	PoolSize string `yaml:"pool_size"`
}
