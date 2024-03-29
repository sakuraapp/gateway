package config

type envType string

const (
	EnvDEV  envType = "DEV"
	EnvPROD envType = "PROD"
)

type Config struct {
	Env  envType
	Port int
	NodeId string
	AllowedOrigins []string
	GrpcPort int
	GrpcCertPath string
	GrpcKeyPath string
	JWTPublicPath string
	DatabaseUser string
	DatabasePassword string
	DatabaseName string
	RedisAddr string
	RedisPassword string
	RedisDatabase int
	S3Region *string
	S3Bucket *string
	S3Endpoint *string
	S3ForcePathStyle *bool
}

func (c *Config) IsDev() bool {
	return c.Env == EnvDEV
}