package handler

import (
	"context"

	pb "github.com/RogueAlmond70/Muzz/explorepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedExploreServiceServer
}

func (Server) ListLikedYou(context.Context, *pb.ListLikedYouRequest) (*pb.ListLikedYouResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListLikedYou not implemented")
}
func (Server) ListNewLikedYou(context.Context, *pb.ListLikedYouRequest) (*pb.ListLikedYouResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListNewLikedYou not implemented")
}
func (Server) CountLikedYou(context.Context, *pb.CountLikedYouRequest) (*pb.CountLikedYouResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CountLikedYou not implemented")
}
func (Server) PutDecision(context.Context, *pb.PutDecisionRequest) (*pb.PutDecisionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PutDecision not implemented")
}
