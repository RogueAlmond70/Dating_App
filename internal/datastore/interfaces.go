package datastore

import (
	"context"
	"time"

	pb "github.com/RogueAlmond70/Muzz/explorepb"
)

type DBClient interface {
	GetLikes(ctx context.Context, req *pb.ListLikedYouRequest) (resp *pb.ListLikedYouResponse, err error)
	GetNewLikes(ctx context.Context, req *pb.ListLikedYouRequest) (resp *pb.ListLikedYouResponse, err error)
	GetCountLikedYou(ctx context.Context, req *pb.CountLikedYouRequest) (resp *pb.CountLikedYouResponse, err error)
	PutDecision(ctx context.Context, req *pb.PutDecisionRequest) (resp *pb.PutDecisionResponse, err error)
}

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
