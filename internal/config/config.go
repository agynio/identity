package config

import (
	"fmt"
	"os"
)

type Config struct {
	GRPCAddress          string
	DatabaseURL          string
	AuthorizationAddress string
}

func FromEnv() (Config, error) {
	cfg := Config{}
	cfg.GRPCAddress = os.Getenv("GRPC_ADDRESS")
	if cfg.GRPCAddress == "" {
		cfg.GRPCAddress = ":50051"
	}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must be set")
	}
	cfg.AuthorizationAddress = os.Getenv("AUTHORIZATION_ADDRESS")
	if cfg.AuthorizationAddress == "" {
		cfg.AuthorizationAddress = "authorization:50051"
	}
	return cfg, nil
}
