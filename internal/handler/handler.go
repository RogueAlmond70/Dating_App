package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"log"

	pb "github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/datastore"
	redis_cache "github.com/RogueAlmond70/Muzz/internal/datastore/redis"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	pb.UnimplementedExploreServiceServer
	DB    datastore.DBClient
	Cache datastore.Cache
}

func (s Server) ListLikedYou(ctx context.Context, req *pb.ListLikedYouRequest) (*pb.ListLikedYouResponse, error) {
	var cacheKey string
	// If there is no pagination token, or it is blank, then it is a new request so we create a cachekey
	// that indicates page one. Otherwise we decode the pagination token and create a cachekey from it.
	if req.PaginationToken == nil || *req.PaginationToken == "" {
		cacheKey = fmt.Sprintf("explore:likes:%s:page:first", req.RecipientUserId)
	} else {
		cursor, err := DecodeCursor(*req.PaginationToken)
		if err != nil {
			log.Printf("Invalid cursor: %v", err)
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
		cacheKey = fmt.Sprintf("explore:likes:%s:page:after:%d_%d", req.RecipientUserId, cursor.CreatedAt, cursor.ID)
	}

	// With a new request, or a request where for some reason the cache bucket cannot be found, this will fall back to
	// the DB. Otherwise it will retrieve the data.
	if data, err := s.Cache.Get(ctx, cacheKey); err == nil && data != nil {
		var resp pb.ListLikedYouResponse
		if err := proto.Unmarshal(data, &resp); err == nil {
			return &resp, nil
		}
	}

	// Querying the database when there is no cached result.
	response, err := s.DB.GetLikes(ctx, req)
	if err != nil {
		log.Printf("Unable to get likes: %v. Please try again later", err)
		return nil, fmt.Errorf("unable to get likes: %v", err)
	}

	// Cache the response if appropriate (see shouldCache for logic).
	if shouldCache(req) {
		data, _ := proto.Marshal(response)
		_ = s.Cache.Set(ctx, cacheKey, data, redis_cache.ExploreTTL)
	}

	return response, nil
}

func (s Server) ListNewLikedYou(ctx context.Context, req *pb.ListLikedYouRequest) (*pb.ListLikedYouResponse, error) {
	var cacheKey string
	// If there is no pagination token, or it is blank, then it is a new request so we create a cachekey
	// that indicates page one. Otherwise we decode the pagination token and create a cachekey from it.
	if req.PaginationToken == nil || *req.PaginationToken == "" {
		cacheKey = fmt.Sprintf("explore:likes:%s:page:first", req.GetRecipientUserId())
	} else {
		cursor, err := DecodeCursor(*req.PaginationToken)
		if err != nil {
			log.Printf("Invalid cursor: %v", err)
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
		cacheKey = fmt.Sprintf("explore:likes:%s:page:after:%d_%d", req.RecipientUserId, cursor.CreatedAt, cursor.ID)
	}

	// With a new request, or a request where for some reason the cache bucket cannot be found, this will fall back to
	// the DB. Otherwise it will retrieve the data.
	if data, err := s.Cache.Get(ctx, cacheKey); err == nil && data != nil {
		var resp pb.ListLikedYouResponse
		if err := proto.Unmarshal(data, &resp); err == nil {
			return &resp, nil
		}
	}

	response, err := s.DB.GetNewLikes(ctx, req)
	if err != nil {
		log.Printf("Unable to get new likes: %v. Please try again later.", err)
		return nil, fmt.Errorf("unable to get new likes: %v", err)
	}

	// Cache the response if appropriate (see shouldCache for logic).
	if shouldCache(req) {
		data, _ := proto.Marshal(response)
		_ = s.Cache.Set(ctx, cacheKey, data, redis_cache.ExploreTTL)
	}
	return response, nil
}

func (s Server) CountLikedYou(ctx context.Context, req *pb.CountLikedYouRequest) (*pb.CountLikedYouResponse, error) {
	// Create a cacheKey from an identifiable string.
	cacheKey := fmt.Sprintf("explore:countLikedYou:%s", req.GetRecipientUserId())

	// Check the cache for a bucket matching the key, return data if found.
	if data, err := s.Cache.Get(ctx, cacheKey); err == nil && data != nil {
		var resp pb.CountLikedYouResponse
		if err := proto.Unmarshal(data, &resp); err == nil {
			return &resp, nil
		}
	}

	// Query the database if nothing is cached for this request.
	response, err := s.DB.GetCountLikedYou(ctx, req)
	if err != nil {
		log.Printf("Unable to get count of likes: %v. Please try again later.", err)
		return nil, fmt.Errorf("unable to get count of likes: %v", err)
	}

	// Cache the response. The TTL can be a bit longer than 10 seconds here and not impact user experience.
	data, _ := proto.Marshal(response)
	_ = s.Cache.Set(ctx, cacheKey, data, redis_cache.ExploreTTL)

	return response, nil
}

func (s Server) PutDecision(ctx context.Context, req *pb.PutDecisionRequest) (*pb.PutDecisionResponse, error) {
	// Create a cacheKey from an identifiable string.
	cacheKey := fmt.Sprintf("explore:mutualLikes:%s:actorid:%s", req.GetRecipientUserId(), req.GetActorUserId())

	// Check the cache for a bucket matching the key, return data if found.
	if data, err := s.Cache.Get(ctx, cacheKey); err == nil && data != nil {
		var resp pb.PutDecisionResponse
		if err := proto.Unmarshal(data, &resp); err == nil {
			return &resp, nil
		}
	}

	// Query the database if nothing is cached for this request.
	response, err := s.DB.PutDecision(ctx, req)
	if err != nil {
		log.Printf("Unable to set decision request: %v. Please try again later.", err)
		return nil, fmt.Errorf("unable to set decision request: %v", err)
	}

	// Cache the response.
	data, _ := proto.Marshal(response)
	_ = s.Cache.Set(ctx, cacheKey, data, redis_cache.ExploreTTL)

	return response, nil
}

func shouldCache(req *pb.ListLikedYouRequest) bool {
	if req.PaginationToken == nil || *req.PaginationToken == "" {
		return true // Cache page 1
	}

	cursor, err := DecodeCursor(*req.PaginationToken)
	if err != nil {
		return false // Bad token, don't cache.
	}

	// If likes are from the last 7 days, cache them.
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	if time.Unix(int64(cursor.CreatedAt), 0).After(cutoff) {
		return true
	} else {
		return false
	}
}

func EncodeCursor(id int, createdAt uint64) (string, error) {
	cursor := PaginationCursor{
		ID:        id,
		CreatedAt: createdAt,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func DecodeCursor(token string) (*PaginationCursor, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var cursor PaginationCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, err
	}
	if err := validateCursor(&cursor); err != nil {
		return nil, err
	}
	return &cursor, nil
}

func validateCursor(c *PaginationCursor) error {
	if c.ID < 0 {
		return fmt.Errorf("invalid cursor: ID cannot be negative")
	}
	if c.CreatedAt == 0 {
		return fmt.Errorf("invalid cursor: CreatedAt timestamp is zero")
	}
	now := uint64(time.Now().Unix())
	if c.CreatedAt > now+3600 { // One hour
		return fmt.Errorf("invalid cursor: CreatedAt timestamp is in the future")
	}
	return nil
}

type PaginationCursor struct {
	ID        int    `json:"id"`
	CreatedAt uint64 `json:"created_at"`
}
