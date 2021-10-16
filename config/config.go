package config

type Config struct {
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