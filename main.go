package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/sakuraapp/gateway/config"
	"github.com/sakuraapp/gateway/server"
	"log"
	"os"
	"strconv"
	"strings"
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

	allowedOrigins := strings.Split(strings.ToLower(os.Getenv("ALLOWED_ORIGINS")), ", ")

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

	s := server.New(config.Config{
		Port:           port,
		NodeId: nodeId,
		AllowedOrigins: allowedOrigins,
		JWTPublicPath: jwtPublicPath,
		DatabaseUser: os.Getenv("DB_USER"),
		DatabasePassword: os.Getenv("DB_PASSWORD"),
		DatabaseName: os.Getenv("DB_DATABASE"),
		RedisAddr: redisAddr,
		RedisPassword: redisPassword,
		RedisDatabase: redisDb,
	})

	if err := s.Start(); err != nil {
		fmt.Printf("Failed to start server: %v", err)
	}
}