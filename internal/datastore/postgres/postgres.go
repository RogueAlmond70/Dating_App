package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/datastore"
	"github.com/RogueAlmond70/Muzz/internal/handler"
	"github.com/RogueAlmond70/Muzz/internal/metrics"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ datastore.DBClient = &Postgres{}

type Postgres struct {
	logger *zap.Logger
	db     *sql.DB
}

func NewPostgres(logger *zap.Logger, sqldb *sql.DB) *Postgres {
	return &Postgres{
		logger: logger,
		db:     sqldb,
	}
}

func ConnectDB() (*sql.DB, error) {
	connStr := "postgres://postgres:password@localhost:5432/myappdb?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error checking database connection: %v", err)
	}
	return db, nil
}

func (p *Postgres) GetLikes(ctx context.Context, req *explorepb.ListLikedYouRequest) (*explorepb.ListLikedYouResponse, error) {
	timer := prometheus.NewTimer(metrics.ListLikedYouRequestDurationSeconds)
	metrics.ListLikedYouRequestsTotal.WithLabelValues().Inc()

	defer func() { // If performance is slow for some reason, it should be logged.
		elapsed := timer.ObserveDuration()
		if elapsed > 500*time.Millisecond {
			p.logger.Warn("slow query detected (>500ms)",
				zap.Duration("duration", elapsed),
				zap.String("method", "GetLikes"),
				zap.String("recipientID", req.RecipientUserId),
			)
		}
	}()

	const pageSize = 25

	var afterCreatedAt time.Time
	var afterID int
	var hasCursor bool

	if req.PaginationToken != nil && *req.PaginationToken != "" {
		cursor, err := handler.DecodeCursor(*req.PaginationToken)
		if err != nil {
			p.logger.Error("unable to decode cursor",
				zap.String("pagination_token", req.GetPaginationToken()),
				zap.Error(err),
				zap.String("operation_name", "ListLikedYou"),
				zap.String("recipientID", req.GetRecipientUserId()),
			)

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
  WHERE liked_id = $1 AND decision = true
  ORDER BY liker_id, created_at DESC, id DESC
)
SELECT liker_id, created_at, id
FROM latest_likes
`

	args := []interface{}{req.RecipientUserId}

	if hasCursor {
		p.logger.Debug("incoming pagination cursor",
			zap.String("operation_name", "ListLikedYou"),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.Time("after_created_at", afterCreatedAt),
			zap.Int("after_id", afterID),
		)

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
		p.logger.Error("unable to run sql query",
			zap.String("operation_name", "ListLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("unable to run sql query: %v", err)
	}
	defer rows.Close()

	var likers []*explorepb.ListLikedYouResponse_Liker
	var nextCursor *handler.PaginationCursor

	for rows.Next() {
		var (
			likerID   string
			createdAt time.Time
			id        int
		)

		if err := rows.Scan(&likerID, &createdAt, &id); err != nil {
			metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("scan_error").Inc()
			p.logger.Error("unable to scan rows",
				zap.String("operation_name", "ListLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
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
			nextCursor = &handler.PaginationCursor{ID: id, CreatedAt: unixTimestamp}
		}
	}
	// Checking for any iteration related errors, connection issues etc
	if err := rows.Err(); err != nil {
		metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("row_iteration_error").Inc()
		p.logger.Error("row iteration error",
			zap.String("operation_name", "ListLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	var nextToken string
	if nextCursor != nil {
		nextToken, err = handler.EncodeCursor(nextCursor.ID, nextCursor.CreatedAt)
		p.logger.Debug("next pagination cursor set",
			zap.String("operation_name", "ListLikedYou"),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.Uint64("next_cursor_created_at", nextCursor.CreatedAt),
			zap.Int("next_cursor_id", nextCursor.ID),
			zap.String("next token", nextToken),
		)

		if err != nil {
			metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("cursor_encoding_error").Inc()
			p.logger.Error("unable to encode cursor for next token",
				zap.String("operation_name", "ListLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
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
	metrics.ListNewLikedYouRequestsTotal.WithLabelValues().Inc()

	defer func() { // If performance is slow for some reason, it should be logged.
		elapsed := timer.ObserveDuration()
		if elapsed > 500*time.Millisecond {
			p.logger.Warn("slow query detected (>500ms)",
				zap.Duration("duration", elapsed),
				zap.String("method", "GetNewLikes"),
				zap.String("recipientID", req.RecipientUserId),
			)
		}
	}()

	const pageSize = 25

	var afterCreatedAt time.Time
	var afterID int
	var hasCursor bool

	if req.PaginationToken != nil && *req.PaginationToken != "" {
		cursor, err := handler.DecodeCursor(*req.PaginationToken)
		if err != nil {
			p.logger.Error("unable to decode cursor",
				zap.String("pagination_token", req.GetPaginationToken()),
				zap.Error(err),
				zap.String("recipient", req.GetRecipientUserId()),
			)
			return nil, status.Errorf(codes.InvalidArgument, "invalid pagination token")
		}
		afterCreatedAt = time.Unix(int64(cursor.CreatedAt), 0)
		afterID = cursor.ID
		hasCursor = true
	}

	query := `
	SELECT l.liker_id, l.created_at, l.id 
	FROM likes l
	WHERE l.liked_id = $1 AND decision = true
	  AND NOT EXISTS (
	    SELECT 1
	    FROM likes l2
	    WHERE l2.liker_id = $1
	      AND l2.liked_id = l.liker_id
	      AND l2.decision = true
	  )
`

	args := []interface{}{req.RecipientUserId}

	if hasCursor {
		p.logger.Debug("incoming pagination cursor",
			zap.String("operation_name", "ListNewLikedYou"),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.Time("after_created_at", afterCreatedAt),
			zap.Int("after_id", afterID),
		)

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
		p.logger.Error("unable to run sql query",
			zap.String("operation_name", "ListNewLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("unable to run sql query: %v", err)
	}
	defer rows.Close()

	var likers []*explorepb.ListLikedYouResponse_Liker
	var nextCursor *handler.PaginationCursor

	for rows.Next() {
		var (
			likerID   string
			createdAt time.Time
			id        int
		)

		if err := rows.Scan(&likerID, &createdAt, &id); err != nil {
			metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("scan_error").Inc()
			p.logger.Error("unable to scan rows",
				zap.String("operation_name", "ListNewLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
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
			nextCursor = &handler.PaginationCursor{ID: id, CreatedAt: unixTimestamp}
		}
	}
	// Checking for any iteration related errors, connection issues etc
	if err := rows.Err(); err != nil {
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("row_iteration_error").Inc()
		p.logger.Error("row iteration error",
			zap.String("operation_name", "ListNewLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	var nextToken string
	if nextCursor != nil {
		nextToken, err = handler.EncodeCursor(nextCursor.ID, nextCursor.CreatedAt)
		p.logger.Debug("next pagination cursor set",
			zap.String("operation_name", "ListNewLikedYou"),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.Uint64("next_cursor_created_at", nextCursor.CreatedAt),
			zap.Int("next_cursor_id", nextCursor.ID),
			zap.String("next token", nextToken),
		)
		if err != nil {
			metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("cursor_encoding_error").Inc()
			p.logger.Error("unable to encode cursor for next token",
				zap.String("operation_name", "ListLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
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
	timer := prometheus.NewTimer(metrics.CountLikedYouRequestDurationSeconds)
	metrics.CountLikedYouRequestsTotal.WithLabelValues().Inc()

	defer func() {
		elapsed := timer.ObserveDuration() // If performance is slow for some reason, it should be logged.
		if elapsed > 500*time.Millisecond {
			p.logger.Warn("slow query detected (>500ms)",
				zap.Duration("duration", elapsed),
				zap.String("method", "GetCountLikedYou"),
				zap.String("recipientID", req.RecipientUserId),
			)
		}
	}()

	query := `
	SELECT COUNT(DISTINCT l.liker_id)
    FROM likes l
    WHERE l.liked_id = $1 AND decision
    AND NOT EXISTS (
    SELECT 1
    FROM likes l2
    WHERE l2.liker_id = $1
      AND l2.liked_id = l.liker_id
      AND l2.decision = true
  )`

	var count int64
	err = p.db.QueryRowContext(ctx, query, req.RecipientUserId).Scan(&count)
	if err != nil {
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("sql_query_error").Inc()
		p.logger.Error("unable to run sql query",
			zap.String("operation_name", "CountLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("unable to run count likes query: %w", err)
	}

	return &explorepb.CountLikedYouResponse{Count: uint64(count)}, nil

}

// Record the decision of the actor to like or pass the recipient
func (p *Postgres) PutDecision(ctx context.Context, req *explorepb.PutDecisionRequest) (resp *explorepb.PutDecisionResponse, err error) {
	timer := prometheus.NewTimer(metrics.PutDecisionRequestDurationSeconds)
	metrics.PutDecisionRequestsTotal.WithLabelValues("request").Inc()

	defer func() {
		elapsed := timer.ObserveDuration() // If performance is slow for some reason, it should be logged.
		if elapsed > 500*time.Millisecond {
			p.logger.Warn("slow query detected (>500ms)",
				zap.Duration("duration", elapsed),
				zap.String("method", "PutDecision"),
				zap.String("recipientID", req.RecipientUserId),
			)
		}
	}()

	query := `
		INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (liker_id, liked_id)
		DO UPDATE SET decision = EXCLUDED.decision, decision_at = EXCLUDED.decision_at
		RETURNING decision
		`

	_, err = p.db.ExecContext(ctx, query, req.ActorUserId, req.RecipientUserId, req.LikedRecipient)
	if err != nil {
		metrics.PutDecisionRequestErrorsTotal.WithLabelValues("insert_update_error").Inc()
		p.logger.Error("unable to run sql query",
			zap.String("operation_name", "PutDecision"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.String("actorID", req.GetActorUserId()),
		)
		return nil, fmt.Errorf("failed to insert or update like decision: %v", err)
	}

	p.logger.Info("put decision recorded",
		zap.String("recipientID", req.GetRecipientUserId()),
		zap.String("actorID", req.GetActorUserId()),
		zap.Bool("liked_recipient", req.GetLikedRecipient()),
	)

	// Check for mutual like only if this user liked the recipient
	mutualLikes := false
	if req.LikedRecipient {
		checkMutualQuery := `
		SELECT TRUE
		FROM likes
		WHERE liker_id = $1 AND liked_id = $2 AND decision
		LIMIT 1
	`
		var exists bool
		err := p.db.QueryRowContext(ctx, checkMutualQuery, req.ActorUserId, req.RecipientUserId).Scan(&exists)

		if errors.Is(err, sql.ErrNoRows) { // No Rows is not necesarily an error, but it does mean there is no mutual like.
			metrics.PutDecisionNoMutualLikesTotal.WithLabelValues("no_mutual_like").Inc()
			p.logger.Info("no mutual like",
				zap.String("recipientID", req.GetRecipientUserId()),
				zap.String("actorID", req.GetActorUserId()),
			)
			return &explorepb.PutDecisionResponse{
				MutualLikes: false,
			}, nil
		} else if err != nil {
			metrics.PutDecisionRequestErrorsTotal.WithLabelValues("general_error").Inc()
			p.logger.Error("unable to run sql query",
				zap.String("operation_name", "PutDecision"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
				zap.String("actorID", req.GetActorUserId()),
			)
			return nil, fmt.Errorf("error checking for mutual like: %v", err)
		}
		mutualLikes = true
		metrics.PutDecisionMutualLikesTotal.WithLabelValues("mutual_like").Inc()
		p.logger.Info("mutual like!",
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.String("actorID", req.GetActorUserId()),
		)
	}
	return &explorepb.PutDecisionResponse{
		MutualLikes: mutualLikes,
	}, nil
}
