package main

import (
	"log"
	"net"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/database/postgres"
	"github.com/RogueAlmond70/Muzz/internal/handler"
	"google.golang.org/grpc"
)

func main() {
	db, err := postgres.ConnectDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	pgClient := postgres.NewPostgres(*log.Default(), db)

	srv := &handler.Server{
		DB: pgClient,
	}

	lis, err := net.Listen("tcp", ":9001")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()

	explorepb.RegisterExploreServiceServer(grpcServer, srv)
	log.Println("gRPC server listening on :9001")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
