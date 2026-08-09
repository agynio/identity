package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	authorizationv1 "github.com/agynio/identity/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/identity/internal/config"
	"github.com/agynio/identity/internal/db"
	"github.com/agynio/identity/internal/server"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("identity-service: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create connection pool: %w", err)
	}
	defer pool.Close()

	if err := db.ApplyMigrations(ctx, pool); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	authConn, err := grpc.NewClient(cfg.AuthorizationAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to authorization: %w", err)
	}
	defer authConn.Close()

	identityStore := store.New(pool)
	authorizationClient := authorizationv1.NewAuthorizationServiceClient(authConn)

	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, server.New(identityStore, authorizationClient))

	// The platform admin identity comes from configuration because the
	// controller that provisions everything else authenticates as it, so
	// nothing can create it through an API. This service owns the record and
	// the cluster admin relation, and converges both in the background --
	// nothing here waits on it, and every request is served meanwhile.
	if cfg.PlatformIdentityID != "" {
		platformIdentityID, err := uuid.Parse(cfg.PlatformIdentityID)
		if err != nil {
			return fmt.Errorf("PLATFORM_IDENTITY_ID: %w", err)
		}
		go server.NewPlatformIdentity(identityStore, authorizationClient).
			EnsureWithRetry(ctx, platformIdentityID, func(format string, args ...any) {
				log.Printf(format, args...)
			})
	}

	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.GRPCAddress, err)
	}

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("IdentityService listening on %s", cfg.GRPCAddress)

	if err := grpcServer.Serve(lis); err != nil {
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
