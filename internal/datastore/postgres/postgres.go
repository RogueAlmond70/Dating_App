package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/config"
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
	cfg    config.Config
}

func NewPostgres(logger *zap.Logger, sqldb *sql.DB, cfg config.Config) *Postgres {
	return &Postgres{
		logger: logger,
		db:     sqldb,
		cfg:    cfg,
	}
}

func getPostgresConnStr(cfg *config.Config) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresDB,
	)
}

func ConnectDB(cfg config.Config) (*sql.DB, error) {
	connStr := getPostgresConnStr(&cfg)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error checking database connection: %w", err)
	}
	return db, nil
}

func ConnectDBWithRetry(cfg config.Config, zlog zap.Logger) (*sql.DB, error) {
	var db *sql.DB
	var err error
	for i := 0; i < cfg.PostgresRetry; i++ {
		db, err = ConnectDB(cfg)
		if err == nil {
			return db, nil
		}
		zlog.Warn("Failed to connect to DB",
			zap.Int("attempt", i+1),
			zap.Int("max_attempts", cfg.PostgresRetry),
			zap.Error(err),
		)
		time.Sleep(cfg.PostgresDelay)
	}
	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", cfg.PostgresRetry, err)
}

// GetLikes implements the ListLikedYouRequest.
func (p *Postgres) GetLikes(ctx context.Context, req *explorepb.ListLikedYouRequest) (*explorepb.ListLikedYouResponse, error) {
	timer := prometheus.NewTimer(metrics.ListLikedYouRequestDurationSeconds)
	metrics.ListLikedYouRequestsTotal.WithLabelValues("list_liked_you_requests_total").Inc()

	defer func() { // If performance is slow for some reason, it should be logged. An additional option is to define a new metric in prometheus and set up an alert.
		elapsed := timer.ObserveDuration()
		if elapsed > 500*time.Millisecond {
			p.logger.Warn("slow query detected (>500ms)",
				zap.Duration("duration", elapsed),
				zap.String("method", "GetLikes"),
				zap.String("recipientID", req.RecipientUserId),
			)
		}
	}()

	pageSize := p.cfg.PageSize // PageSize is set to 25 by default in the environment variables.

	var afterCreatedAt time.Time
	var afterID int
	var hasCursor bool

	if req.PaginationToken != nil && *req.PaginationToken != "" { // If a non blank pagination token exists, we decode it.
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

	// This query retrieves likes where the liker is the actor and the liked is the recipient. I've limited results to the past year,
	// but other options are feasible, for example having an archive table where likes for a given user past a certain date or quantity,
	//  are automatically saved to. This could be maintained with a cron job.
	query := `   
	WITH latest_likes AS (
	SELECT DISTINCT ON (liker_id) liker_id, created_at, id
	FROM likes
	WHERE liked_id = $1 AND decision = true
	AND created_at >= NOW() - INTERVAL '1 year'
	ORDER BY liker_id, created_at DESC, id DESC
	)
	SELECT liker_id, created_at, id
	FROM latest_likes 
	`

	// This will hold the arguments we pass in to complete the query.
	args := []interface{}{req.RecipientUserId}

	if hasCursor {
		p.logger.Debug("incoming pagination cursor",
			zap.String("operation_name", "ListLikedYou"),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.Time("after_created_at", afterCreatedAt),
			zap.Int("after_id", afterID),
		)

		query += " WHERE (created_at, id) < ($2, $3)" // If there is a cursor / pagination token, we use the userID and createdAt values in the WHERE clause.
		args = append(args, afterCreatedAt.UTC(), afterID)
		query += " ORDER BY created_at DESC, id DESC LIMIT $4" // The limit will be the pageSize, but when we have a cursor that is the 4th argument, and when we don't it is the 2nd.
		args = append(args, pageSize+1)
	} else {
		query += " ORDER BY created_at DESC, id DESC LIMIT $2"
		args = append(args, pageSize+1)
	}

	rows, err := p.db.QueryContext(ctx, query, args...) // Retrieve the rows of results.
	if err != nil {
		metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("list_liked_you_requests_total").Inc()
		p.logger.Error("unable to run sql query",
			zap.String("operation_name", "ListLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("unable to run sql query: %w", err)
	}
	defer rows.Close()

	var likers []*explorepb.ListLikedYouResponse_Liker
	var nextCursor *handler.PaginationCursor

	for rows.Next() { // Here we loop through the rows, populating the likerID, createdAt, and id variables.
		var (
			likerID   string
			createdAt time.Time
			id        int
		)

		if err := rows.Scan(&likerID, &createdAt, &id); err != nil {
			metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("list_liked_you_requests_total").Inc()
			p.logger.Error("unable to scan rows",
				zap.String("operation_name", "ListLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
			return nil, fmt.Errorf("unable to scan rows: %w", err)
		}

		unixTimestamp := uint64(createdAt.Unix())

		if len(likers) < pageSize {
			likers = append(likers, &explorepb.ListLikedYouResponse_Liker{
				ActorId:       likerID,
				UnixTimestamp: unixTimestamp,
			})
		} else {
			// We've hit one extra result so set next cursor.
			nextCursor = &handler.PaginationCursor{ID: id, CreatedAt: unixTimestamp}
		}
	}
	// Checking for any iteration related errors, connection issues etc.
	if err := rows.Err(); err != nil {
		metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("list_liked_you_requests_total").Inc()
		p.logger.Error("row iteration error",
			zap.String("operation_name", "ListLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("row iteration error: %w", err)
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
			metrics.ListLikedYouRequestErrorsTotal.WithLabelValues("list_liked_you_requests_total").Inc()
			p.logger.Error("unable to encode cursor for next token",
				zap.String("operation_name", "ListLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
			return nil, fmt.Errorf("unable to get next token: %w", err)
		}
	}

	return &explorepb.ListLikedYouResponse{
		Likers:              likers,
		NextPaginationToken: &nextToken,
	}, nil
}

// GetNewLikes implements ListNewLikedYourRequest, and lists all users who liked the recipient excluding those who have been liked in return.
func (p *Postgres) GetNewLikes(ctx context.Context, req *explorepb.ListLikedYouRequest) (resp *explorepb.ListLikedYouResponse, err error) {
	timer := prometheus.NewTimer(metrics.ListNewLikedYouRequestDurationSeconds)
	metrics.ListNewLikedYouRequestsTotal.WithLabelValues("list_new_liked_you_requests_total").Inc()

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

	pageSize := p.cfg.PageSize // PageSize is set to 25 by default in the environment variables.

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

	// This query selects all the actors that have liked the recipient that the recipient hasn't liked back at this point in time.
	// Again I've limited returned users to those who liked the recipient within the past year.
	query := `
		SELECT l.liker_id, l.created_at, l.id        
		FROM likes l
		WHERE l.liked_id = $1 AND decision = true
		AND created_at >= NOW() - INTERVAL '1 year'

		AND NOT EXISTS (
			SELECT 1
			FROM likes l2
			WHERE l2.liker_id = $1
			AND l2.liked_id = l.liker_id
			AND l2.decision = true
			AND created_at >= NOW() - INTERVAL '1 year'

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
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("list_new_liked_you_requests_total").Inc()
		p.logger.Error("unable to run sql query",
			zap.String("operation_name", "ListNewLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("unable to run sql query: %w", err)
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
			metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("list_new_liked_you_requests_total").Inc()
			p.logger.Error("unable to scan rows",
				zap.String("operation_name", "ListNewLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
			return nil, fmt.Errorf("unable to scan rows: %w", err)
		}

		unixTimestamp := uint64(createdAt.Unix())

		if len(likers) < pageSize {
			likers = append(likers, &explorepb.ListLikedYouResponse_Liker{
				ActorId:       likerID,
				UnixTimestamp: unixTimestamp,
			})
		} else {
			// We've hit one extra result so set next cursor.
			nextCursor = &handler.PaginationCursor{ID: id, CreatedAt: unixTimestamp}
		}
	}
	// Checking for any iteration related errors, connection issues etc.
	if err := rows.Err(); err != nil {
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("list_new_liked_you_requests_total").Inc()
		p.logger.Error("row iteration error",
			zap.String("operation_name", "ListNewLikedYou"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
		)
		return nil, fmt.Errorf("row iteration error: %w", err)
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
			metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("list_new_liked_you_requests_total").Inc()
			p.logger.Error("unable to encode cursor for next token",
				zap.String("operation_name", "ListLikedYou"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
			return nil, fmt.Errorf("unable to get next token: %w", err)
		}
	}

	return &explorepb.ListLikedYouResponse{
		Likers:              likers,
		NextPaginationToken: &nextToken,
	}, nil
}

// GetCountLikedYou implements CountLikedYouRequest, returning the count of users who have liked the recipient at the time of requesting.
func (p *Postgres) GetCountLikedYou(ctx context.Context, req *explorepb.CountLikedYouRequest) (resp *explorepb.CountLikedYouResponse, err error) {
	timer := prometheus.NewTimer(metrics.CountLikedYouRequestDurationSeconds)                  // Setting a timer to track how long the method takes.
	metrics.CountLikedYouRequestsTotal.WithLabelValues("count_liked_you_requests_total").Inc() // Increment the counter of total requests of this type.

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
		AND created_at >= NOW() - INTERVAL '1 year'

	)`

	var count int64
	err = p.db.QueryRowContext(ctx, query, req.RecipientUserId).Scan(&count)
	if err != nil {
		metrics.ListNewLikedYouRequestErrorsTotal.WithLabelValues("count_liked_you_requests_total").Inc()
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
	metrics.PutDecisionRequestsTotal.WithLabelValues("put_decision_requests_total").Inc()

	defer func() {
		elapsed := timer.ObserveDuration()
		if elapsed > 500*time.Millisecond {
			p.logger.Warn("slow query detected (>500ms)",
				zap.Duration("duration", elapsed),
				zap.String("method", "PutDecision"),
				zap.String("recipientID", req.GetRecipientUserId()),
			)
		}
	}()

	query := `
		INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (liker_id, liked_id)
		DO UPDATE SET 
			decision = EXCLUDED.decision,
			decision_at = EXCLUDED.decision_at
		WHERE likes.decision IS DISTINCT FROM EXCLUDED.decision
		RETURNING decision
	`

	var newDecision bool
	err = p.db.QueryRowContext(ctx, query, req.ActorUserId, req.RecipientUserId, req.LikedRecipient).Scan(&newDecision)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		p.logger.Info("no-op: decision already recorded",
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.String("actorID", req.GetActorUserId()),
			zap.Bool("liked_recipient", req.GetLikedRecipient()),
		)
	case err != nil:
		metrics.PutDecisionRequestErrorsTotal.WithLabelValues("put_decision_requests_total").Inc()
		p.logger.Error("failed to execute insert/update",
			zap.String("operation_name", "PutDecision"),
			zap.Error(err),
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.String("actorID", req.GetActorUserId()),
		)
		return nil, fmt.Errorf("failed to insert or update like decision: %w", err)
	default:
		p.logger.Info("put decision recorded",
			zap.String("recipientID", req.GetRecipientUserId()),
			zap.String("actorID", req.GetActorUserId()),
			zap.Bool("liked_recipient", req.GetLikedRecipient()),
		)
	}

	// Check for mutual like only if user liked the recipient
	mutualLikes := false
	if req.LikedRecipient {
		checkMutualQuery := `
			SELECT TRUE
			FROM likes
			WHERE liker_id = $1 AND liked_id = $2 AND decision = TRUE
			LIMIT 1
		`
		var exists bool
		err := p.db.QueryRowContext(ctx, checkMutualQuery, req.RecipientUserId, req.ActorUserId).Scan(&exists)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			metrics.PutDecisionNoMutualLikesTotal.WithLabelValues("no_mutual_like").Inc()
			p.logger.Info("no mutual like",
				zap.String("recipientID", req.GetRecipientUserId()),
				zap.String("actorID", req.GetActorUserId()),
			)
		case err != nil:
			metrics.PutDecisionRequestErrorsTotal.WithLabelValues("put_decision_requests_total").Inc()
			p.logger.Error("error checking mutual like",
				zap.String("operation_name", "PutDecision"),
				zap.Error(err),
				zap.String("recipientID", req.GetRecipientUserId()),
				zap.String("actorID", req.GetActorUserId()),
			)
			return nil, fmt.Errorf("error checking for mutual like: %w", err)
		default:
			mutualLikes = true
			metrics.PutDecisionMutualLikesTotal.WithLabelValues("mutual_like").Inc()
			p.logger.Info("mutual like!",
				zap.String("recipientID", req.GetRecipientUserId()),
				zap.String("actorID", req.GetActorUserId()),
			)
		}
	}

	return &explorepb.PutDecisionResponse{
		MutualLikes: mutualLikes,
	}, nil
}
