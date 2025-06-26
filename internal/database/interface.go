package database

import (
	"context"
)

type DBClient interface {
	GetLikes(ctx context.Context, recipientUserID string) (actorIDs []string, err error)
	GetNewLikes(ctx context.Context, recipientUserID string) (actorIDs []string, err error)
	GetCountLikedYou(ctx context.Context, recipientUserID string) (countLikedYou int, err error)
	PutDecisionResponse(ctx context.Context, actorUserID string, RecipientUserID string) (decision bool, err error)
}

/*
service ExploreService {
  rpc ListLikedYou(ListLikedYouRequest) returns (ListLikedYouResponse); // List all users who liked the recipient
  rpc ListNewLikedYou(ListLikedYouRequest) returns (ListLikedYouResponse); // List all users who liked the recipient excluding those who have been liked in return
  rpc CountLikedYou(CountLikedYouRequest) returns (CountLikedYouResponse); // Count the number of users who liked the recipient
  rpc PutDecision(PutDecisionRequest) returns (PutDecisionResponse); // Record the decision of the actor to like or pass the recipient
}
*/
