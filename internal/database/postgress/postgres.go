package postgres

import (
	"context"
	"database/sql"
	"log"
	"time"

	"encoding/base64"
	"encoding/json"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/database"
	_ "github.com/lib/pq"
)

type Postgres struct {
	logger         log.Logger
	databaseClient database.DBClient
}

func NewPostgres(logger log.Logger, databaseClient database.DBClient) *Postgres {
	return &Postgres{
		logger:         logger,
		databaseClient: databaseClient,
	}
}

func connectDB() (*sql.DB, error) {
	connStr := "postgres://postgres:password@localhost:5432/myappdb?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	// Optional: ping to verify connection
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

type PaginationCursor struct {
	ID        int       `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// This will be called by ListLikedYou
func (p *Postgres) GetLikes(ctx context.Context, req *explorepb.ListLikedYouRequest) (resp *explorepb.ListLikedYouResponse, err error) {
	const pageSize = 25

	var afterCreatedAt time.Time
	var afterID int
	var hasCursor bool

	if re.PaginationToken != "" {
		cursor, err := decodeCursor(req.PaginationToken)
	}

	query :=
		` SELECT liker_id, created_at, id
	FROM likes
	WHERE liked_id = $1
	AND (created_at, d <$2, $3)
	ORDER BY created_at DESC, id DESC
	LIMIT 25;`

}

// This will be called by ListNewLikedYou
func (p *Postgres) GetNewLikes(ctx context.Context, re *explorepb.ListLikedYouRequest) (resp *explorepb.ListLikedYouResponse, err error) {

}

// This will be called bu CountLikedYou

// This will be called by PutDecision

/*
func (UnimplementedExploreServiceServer) ListLikedYou(context.Context, *ListLikedYouRequest) (*ListLikedYouResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListLikedYou not implemented")
}
func (UnimplementedExploreServiceServer) ListNewLikedYou(context.Context, *ListLikedYouRequest) (*ListLikedYouResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListNewLikedYou not implemented")
}
func (UnimplementedExploreServiceServer) CountLikedYou(context.Context, *CountLikedYouRequest) (*CountLikedYouResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CountLikedYou not implemented")
}
func (UnimplementedExploreServiceServer) PutDecision(context.Context, *PutDecisionRequest) (*PutDecisionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PutDecision not implemented")




type DBClient interface {
	GetLikes(ctx context.Context, recipientUserID string) (actorIDs []string, err error)
	GetNewLikes(ctx context.Context, recipientUserID string) (actorIDs []string, err error)
	GetCountLikedYou(ctx context.Context, recipientUserID string) (countLikedYou int, err error)
	PutDecisionResponse(ctx context.Context, actorUserID string, RecipientUserID string) (decision bool, err error)
}
*/

func encodeCursor(id int, createdAt time.Time) (string, error) {
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
func decodeCursor(token string) (*PaginationCursor, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var cursor PaginationCursor
	err = json.Unmarshal(data, &cursor)
	return &cursor, err
}

/* Protobuf definition
message PaginationToken {
  int64 created_at_unix_nano = 1;  // Use UnixNano for precise timestamp
  int64 last_id = 2;               // The last ID from the previous page
}
*/
