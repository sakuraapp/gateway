package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/sakuraapp/gateway/server"
	"log"
	"os"
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

	jwtPublicPath := os.Getenv("JWT_PUBLIC_KEY")

	s := server.New(server.Config{
		Port:           port,
		AllowedOrigins: allowedOrigins,
		JWTPublicPath: jwtPublicPath,
	})

	if err := s.Start(); err != nil {
		fmt.Printf("Failed to start server: %v", err)
	}
}