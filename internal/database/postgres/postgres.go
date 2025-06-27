package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"encoding/base64"
	"encoding/json"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/database"
	"github.com/RogueAlmond70/Muzz/internal/metrics"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ database.DBClient = &Postgres{}

type Postgres struct {
	logger log.Logger
	db     *sql.DB
}

func NewPostgres(logger log.Logger, sqldb *sql.DB) *Postgres {
	return &Postgres{
		logger: logger,
		db:     sqldb,
	}
}

func ConnectDB() (*sql.DB, error) {
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
	ID        int    `json:"id"`
	CreatedAt uint64 `json:"created_at"`
}

// This will be called by ListLikedYou, and needs to be called repeatedly o as not to overwhelm device
func (p *Postgres) GetLikes(ctx context.Context, req *explorepb.ListLikedYouRequest) (*explorepb.ListLikedYouResponse, error) {
	timer := prometheus.NewTimer(metrics.ListLikedYouRequestDurationSeconds)
	defer timer.ObserveDuration()
	metrics.ListLikedYouRequestsTotal.WithLabelValues().Inc()

	const pageSize = 25

	var afterCreatedAt time.Time
	var afterID int
	var hasCursor bool

	if req.PaginationToken != nil && *req.PaginationToken != "" {
		cursor, err := decodeCursor(*req.PaginationToken)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid pagination token")
		}
		afterCreatedAt = time.Unix(int64(cursor.CreatedAt), 0)
		afterID = cursor.ID
		hasCursor = true
	}

	query := `
WITH latest_likes AS (
  SELECT DISTINCT ON (liker_id) liker_id, created_at, id
  FROM likes
  WHERE liked_id = $1
  ORDER BY liker_id, created_at DESC, id DESC
)
SELECT liker_id, created_at, id
FROM latest_likes
`

	args := []interface{}{req.RecipientUserId}

	if hasCursor {
		query += " WHERE (created_at, id) < ($2, $3)"
		args = append(args, afterCreatedAt.UTC(), afterID)
		query += " ORDER BY created_at DESC, id DESC LIMIT $4"
		args = append(args, pageSize+1)
	} else {
		query += " ORDER BY created_at DESC, id DESC LIMIT $2"
		args = append(args, pageSize+1)
	}

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("sql_query_error").Inc()
		return nil, fmt.Errorf("unable to run sql query: %v", err)
	}
	defer rows.Close()

	var likers []*explorepb.ListLikedYouResponse_Liker
	var nextCursor *PaginationCursor

	for rows.Next() {
		var (
			likerID   string
			createdAt time.Time
			id        int
		)

		if err := rows.Scan(&likerID, &createdAt, &id); err != nil {
			metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("scan_error").Inc()
			return nil, fmt.Errorf("unable to scan rows: %v", err)
		}

		unixTimestamp := uint64(createdAt.Unix())

		if len(likers) < pageSize {
			likers = append(likers, &explorepb.ListLikedYouResponse_Liker{
				ActorId:       likerID,
				UnixTimestamp: unixTimestamp,
			})
		} else {
			// We've hit one extra result → set next cursor
			nextCursor = &PaginationCursor{ID: id, CreatedAt: unixTimestamp}
		}
	}
	// Checking for any iteration related errors, connection issues etc
	if err := rows.Err(); err != nil {
		metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("row_iteration_error").Inc()
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	var nextToken string
	if nextCursor != nil {
		nextToken, err = encodeCursor(nextCursor.ID, nextCursor.CreatedAt)
		if err != nil {
			metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("cursor_encoding_error").Inc()
			return nil, fmt.Errorf("unable to get next token: %v", err)
		}
	}

	return &explorepb.ListLikedYouResponse{
		Likers:              likers,
		NextPaginationToken: &nextToken,
	}, nil
}

// List all users who liked the recipient excluding those who have been liked in return
func (p *Postgres) GetNewLikes(ctx context.Context, req *explorepb.ListLikedYouRequest) (resp *explorepb.ListLikedYouResponse, err error) {
	timer := prometheus.NewTimer(metrics.ListNewLikedYouRequestDurationSeconds)
	defer timer.ObserveDuration()
	metrics.ListNewLikedYouRequestsTotal.WithLabelValues().Inc()

	const pageSize = 25

	var afterCreatedAt time.Time
	var afterID int
	var hasCursor bool

	if req.PaginationToken != nil && *req.PaginationToken != "" {
		cursor, err := decodeCursor(*req.PaginationToken)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid pagination token")
		}
		afterCreatedAt = time.Unix(int64(cursor.CreatedAt), 0)
		afterID = cursor.ID
		hasCursor = true
	}

	query := `
	SELECT l.liker_id, l.created_at, l.id
	FROM likes l
	WHERE l.liked_id = $1
	  AND NOT EXISTS (
	    SELECT 1
	    FROM likes l2
	    WHERE l2.liker_id = $1
	      AND l2.liked_id = l.liker_id
	      AND l2.decision = 'like'
	  )
`

	args := []interface{}{req.RecipientUserId}

	if hasCursor {
		query += " AND (l.created_at, l.id) < ($2, $3)"
		args = append(args, afterCreatedAt.UTC(), afterID)
		query += " ORDER BY l.created_at DESC, l.id DESC LIMIT $4"
		args = append(args, pageSize+1)
	} else {
		query += " ORDER BY l.created_at DESC, l.id DESC LIMIT $2"
		args = append(args, pageSize+1)
	}

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("sql_query_error").Inc()
		return nil, fmt.Errorf("unable to run sql query: %v", err)
	}
	defer rows.Close()

	var likers []*explorepb.ListLikedYouResponse_Liker
	var nextCursor *PaginationCursor

	for rows.Next() {
		var (
			likerID   string
			createdAt time.Time
			id        int
		)

		if err := rows.Scan(&likerID, &createdAt, &id); err != nil {
			metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("scan_error").Inc()
			return nil, fmt.Errorf("unable to scan rows: %v", err)
		}

		unixTimestamp := uint64(createdAt.Unix())

		if len(likers) < pageSize {
			likers = append(likers, &explorepb.ListLikedYouResponse_Liker{
				ActorId:       likerID,
				UnixTimestamp: unixTimestamp,
			})
		} else {
			// We've hit one extra result → set next cursor
			nextCursor = &PaginationCursor{ID: id, CreatedAt: unixTimestamp}
		}
	}
	// Checking for any iteration related errors, connection issues etc
	if err := rows.Err(); err != nil {
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("row_iteration_error").Inc()
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	var nextToken string
	if nextCursor != nil {
		nextToken, err = encodeCursor(nextCursor.ID, nextCursor.CreatedAt)
		if err != nil {
			metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("cursor_encoding_error").Inc()
			return nil, fmt.Errorf("unable to get next token: %v", err)
		}
	}

	return &explorepb.ListLikedYouResponse{
		Likers:              likers,
		NextPaginationToken: &nextToken,
	}, nil
}

// This will be called by CountLikedYou
func (p *Postgres) GetCountLikedYou(ctx context.Context, req *explorepb.CountLikedYouRequest) (resp *explorepb.CountLikedYouResponse, err error) {
	timer := prometheus.NewTimer(metrics.ListNewLikedYouRequestDurationSeconds)
	defer timer.ObserveDuration()
	metrics.ListNewLikedYouRequestsTotal.WithLabelValues().Inc()
	return nil, nil
}

// This will be called by PutDecision
func (p *Postgres) PutDecision(ctx context.Context, req *explorepb.PutDecisionRequest) (resp *explorepb.PutDecisionResponse, err error) {
	return nil, nil
}

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

func encodeCursor(id int, createdAt uint64) (string, error) {
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
