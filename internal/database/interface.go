package database

import (
	"context"

	pb "github.com/RogueAlmond70/Muzz/explorepb"
)

type DBClient interface {
	GetLikes(ctx context.Context, req *pb.ListLikedYouRequest) (resp *pb.ListLikedYouResponse, err error)
	GetNewLikes(ctx context.Context, req *pb.ListLikedYouRequest) (resp *pb.ListLikedYouResponse, err error)
	GetCountLikedYou(ctx context.Context, req *pb.CountLikedYouRequest) (resp *pb.CountLikedYouResponse, err error)
	PutDecision(ctx context.Context, req *pb.PutDecisionRequest) (resp *pb.PutDecisionResponse, err error)
}
