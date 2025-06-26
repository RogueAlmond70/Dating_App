package handler

import (
	"context"
	"fmt"

	"log"

	pb "github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/database"
)

type Server struct {
	pb.UnimplementedExploreServiceServer
	DB database.DBClient
}

func (s Server) ListLikedYou(ctx context.Context, req *pb.ListLikedYouRequest) (*pb.ListLikedYouResponse, error) {
	response, err := s.DB.GetLikes(ctx, req)
	if err != nil {
		log.Printf("Unable to get likes: %v. Please try again later", err)
		return nil, fmt.Errorf("unable to get likes: %v", err)
	}
	return response, nil
}

func (s Server) ListNewLikedYou(ctx context.Context, req *pb.ListLikedYouRequest) (*pb.ListLikedYouResponse, error) {
	response, err := s.DB.GetNewLikes(ctx, req)
	if err != nil {
		log.Printf("Unable to get new likes: %v. Please try again later.", err)
		return nil, fmt.Errorf("unable to get new likes: %v", err)
		// Add prometheus logging
	}
	return response, nil
}

// List all users who liked the recipient excluding those who have been liked in return
func (s Server) CountLikedYou(ctx context.Context, req *pb.CountLikedYouRequest) (*pb.CountLikedYouResponse, error) {
	response, err := s.DB.GetCountLikedYou(ctx, req)
	if err != nil {
		log.Printf("Unable to get count of likes: %v. Please try again later.", err)
		return nil, fmt.Errorf("unable to get count of likes: %v", err)
		// Add prometheus logging
	}
	return response, nil
}

func (s Server) PutDecision(ctx context.Context, req *pb.PutDecisionRequest) (*pb.PutDecisionResponse, error) {
	response, err := s.DB.PutDecision(ctx, req)
	if err != nil {
		log.Printf("Unable to set decision request: %v. Please try again later.", err)
		return nil, fmt.Errorf("unable to set decision request: %v", err)
		// Add prometheus logging
	}
	return response, nil
}
