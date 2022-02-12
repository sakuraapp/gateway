package main

import (
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/sakuraapp/gateway/internal/config"
	"github.com/sakuraapp/gateway/internal/server"
	sharedUtil "github.com/sakuraapp/shared/pkg/util"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
)

func main() {
	err := godotenv.Load()

	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	port := os.Getenv("PORT")

	if port == "" {
		port = "9000"
	}

	env := os.Getenv("APP_ENV")
	envType := config.EnvDEV

	if env == string(config.EnvPROD) {
		envType = config.EnvPROD
	}

	allowedOrigins := sharedUtil.ParseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))

	redisAddr := os.Getenv("REDIS_ADDR")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDatabase := os.Getenv("REDIS_DATABASE")
	redisDb, err := strconv.Atoi(redisDatabase)

	if err != nil {
		redisDb = 0
	}

	jwtPublicPath := os.Getenv("JWT_PUBLIC_KEY")
	nodeId := os.Getenv("NODE_ID")

	if nodeId == "" {
		nodeId = uuid.NewString()
	}

	s3Region := os.Getenv("S3_REGION")
	s3Bucket := os.Getenv("S3_BUCKET")
	s3Endpoint := os.Getenv("S3_ENDPOINT")
	s3ForcePathStyleStr := os.Getenv("S3_FORCE_PATH_STYLE")
	s3ForcePathStyle := false

	if s3ForcePathStyleStr == "1" {
		s3ForcePathStyle = true
	}

	s := server.New(config.Config{
		Env: envType,
		Port: port,
		NodeId: nodeId,
		AllowedOrigins: allowedOrigins,
		JWTPublicPath: jwtPublicPath,
		DatabaseUser: os.Getenv("DB_USER"),
		DatabasePassword: os.Getenv("DB_PASSWORD"),
		DatabaseName: os.Getenv("DB_DATABASE"),
		RedisAddr: redisAddr,
		RedisPassword: redisPassword,
		RedisDatabase: redisDb,
		S3Bucket: &s3Bucket,
		S3Region: &s3Region,
		S3Endpoint: &s3Endpoint,
		S3ForcePathStyle: &s3ForcePathStyle,
	})

	if err := s.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}