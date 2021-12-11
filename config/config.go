package config

type envType string

const (
	EnvDEV envType = "DEV"
	EnvPROD envType = "PROD"
)

type Config struct {
	Env envType
	Port string
	NodeId string
	AllowedOrigins []string
	JWTPublicPath string
	DatabaseUser string
	DatabasePassword string
	DatabaseName string
	RedisAddr string
	RedisPassword string
	RedisDatabase int
}

func (c *Config) IsDev() bool {
	return c.Env == EnvDEV
}