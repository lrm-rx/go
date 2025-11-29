package config

type Config struct {
	System System `yaml:"system"`
	DB     DB     `yaml:"db"`
}
