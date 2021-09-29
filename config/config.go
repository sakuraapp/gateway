package config

type Config struct {
	Port string
	NodeId string
	AllowedOrigins []string
	JWTPublicPath string
	RedisAddr string
	RedisPassword string
	RedisDatabase int
}