package postgres_test

import (
	"context"
	"database/sql"
	"errors"

	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/config"
	"github.com/RogueAlmond70/Muzz/internal/datastore/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupPostgres(t *testing.T) (*postgres.Postgres, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	cfg := config.Config{
		PageSize:      2,
		PostgresRetry: 1,
		PostgresDelay: time.Millisecond,
	}
	logger, _ := zap.NewDevelopment()
	p := postgres.NewPostgres(logger, db, cfg)
	return p, mock, func() {
		db.Close()
		logger.Sync()
	}
}

func TestGetLikes_NoCursor_Success(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user55",
	}
	expectUserExists(mock, "user55", true)

	// Mock rows returned from query
	rows := sqlmock.NewRows([]string{"liker_id", "created_at", "id"}).
		AddRow("likerA", time.Now(), 1).
		AddRow("likerB", time.Now().Add(-time.Minute), 2).
		AddRow("likerC", time.Now().Add(-2*time.Minute), 3) // extra row triggers nextCursor

	mock.ExpectQuery(regexp.QuoteMeta(`
	WITH latest_likes AS (
	SELECT DISTINCT ON (liker_id) liker_id, created_at, id
	FROM likes
	WHERE liked_id = $1 AND decision = true
	AND created_at >= NOW() - INTERVAL '1 year'
	ORDER BY liker_id, created_at DESC, id DESC
	)
	SELECT liker_id, created_at, id
	FROM latest_likes 
	 ORDER BY created_at DESC, id DESC LIMIT $2`)).
		WithArgs(req.RecipientUserId, 3).
		WillReturnRows(rows)

	resp, err := p.GetLikes(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Likers) != 2 {
		t.Errorf("expected 2 likers, got %d", len(resp.Likers))
	}
	if resp.NextPaginationToken == nil || *resp.NextPaginationToken == "" {
		t.Errorf("expected next pagination token to be set")
	}
}
func TestGetLikes_UserNotFound(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "missing-user", // The id isn't actually relevent, the query simulates not finding a user.
	}

	// Simulate user existence check by returning no rows
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM users WHERE id = $1 LIMIT 1`)).
		WithArgs(req.RecipientUserId).
		WillReturnRows(sqlmock.NewRows([]string{"exists"})) // no rows = user not found

	_, err := p.GetLikes(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for missing user, got nil")
	}
	if !errors.Is(err, postgres.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestGetNewLikes_NoCursor_Success(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user55",
	}
	expectUserExists(mock, "user55", true)

	// Expect main query to return 2 likers
	mock.ExpectQuery(regexp.QuoteMeta(`
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
		 ORDER BY l.created_at DESC, l.id DESC LIMIT $2`)).
		WithArgs(req.RecipientUserId, 3). // PageSize + 1
		WillReturnRows(sqlmock.NewRows([]string{"liker_id", "created_at", "id"}).
			AddRow("userA", time.Now(), 10).
			AddRow("userB", time.Now().Add(-time.Minute), 9))

	resp, err := p.GetNewLikes(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Likers) != 2 {
		t.Errorf("expected 2 likers, got %d", len(resp.Likers))
	}
}

func TestGetNewLikes_UserNotFound(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user49",
	}

	// userExists returns no rows (false)
	mock.ExpectQuery(`SELECT 1 FROM users WHERE id = \$1 LIMIT 1`).
		WithArgs(req.RecipientUserId).
		WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := p.GetNewLikes(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for missing user, got none")
	}
	if !errors.Is(err, postgres.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestGetCountLikedYou_Success(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.CountLikedYouRequest{
		RecipientUserId: "user1",
	}
	expectUserExists(mock, "user1", true)

	mock.ExpectQuery(regexp.QuoteMeta(`
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

	)`)).
		WithArgs(req.RecipientUserId).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	resp, err := p.GetCountLikedYou(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 42 {
		t.Errorf("expected count 42, got %d", resp.Count)
	}
}

func TestGetCountLikedYou_UserNotFound(t *testing.T) {
	p, mock, cleanup := setupPostgres(t)
	defer cleanup()

	req := &explorepb.CountLikedYouRequest{
		RecipientUserId: "missing-user",
	}

	// Simulate user existence check
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM users WHERE id = $1 LIMIT 1`)).
		WithArgs(req.RecipientUserId).
		WillReturnError(sql.ErrNoRows)

	_, err := p.GetCountLikedYou(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for missing user, got nil")
	}
	if !errors.Is(err, postgres.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}
func TestPutDecision_NewDecisionMutualLike(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	p := postgres.NewPostgres(zap.NewNop(), db, config.Config{})

	ctx := context.Background()
	actorID := "user1"
	recipientID := "user12"

	// Both users exist
	expectUserExists(mock, recipientID, true)
	expectUserExists(mock, actorID, true)

	// Expect insert (or update) decision returns the new decision (true)
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (liker_id, liked_id)
		DO UPDATE SET 
			decision = EXCLUDED.decision,
			decision_at = EXCLUDED.decision_at
		WHERE likes.decision IS DISTINCT FROM EXCLUDED.decision
		RETURNING decision
	`)).
		WithArgs(actorID, recipientID, true).
		WillReturnRows(sqlmock.NewRows([]string{"decision"}).AddRow(true))

	// Mutual like check returns true -> mutual like found
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT TRUE
		FROM likes
		WHERE liker_id = $1 AND liked_id = $2 AND decision = TRUE
		LIMIT 1
	`)).
		WithArgs(recipientID, actorID).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(true))

	req := &explorepb.PutDecisionRequest{
		ActorUserId:     actorID,
		RecipientUserId: recipientID,
		LikedRecipient:  true,
	}

	resp, err := p.PutDecision(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.MutualLikes)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestPutDecision_NoMutualLike(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	p := postgres.NewPostgres(zap.NewNop(), db, config.Config{})

	ctx := context.Background()
	actorID := "user1"
	recipientID := "user15"

	// Expect userExists for recipient and actor - both exist
	expectUserExists(mock, recipientID, true)
	expectUserExists(mock, actorID, true)

	// Expect the INSERT ... ON CONFLICT query (simulate insert/update decision)
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (liker_id, liked_id)
		DO UPDATE SET 
			decision = EXCLUDED.decision,
			decision_at = EXCLUDED.decision_at
		WHERE likes.decision IS DISTINCT FROM EXCLUDED.decision
		RETURNING decision
	`)).
		WithArgs(actorID, recipientID, true).
		WillReturnRows(sqlmock.NewRows([]string{"decision"}).AddRow(true))

	// Expect query to check mutual like returns no rows (no mutual like)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT TRUE
		FROM likes
		WHERE liker_id = $1 AND liked_id = $2 AND decision = TRUE
		LIMIT 1
	`)).
		WithArgs(recipientID, actorID).
		WillReturnError(sql.ErrNoRows)

	req := &explorepb.PutDecisionRequest{
		ActorUserId:     actorID,
		RecipientUserId: recipientID,
		LikedRecipient:  true,
	}

	resp, err := p.PutDecision(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.MutualLikes)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func expectUserExists(mock sqlmock.Sqlmock, userID string, exists bool) {
	query := regexp.QuoteMeta(`SELECT 1 FROM users WHERE id = $1 LIMIT 1`)
	if exists {
		mock.ExpectQuery(query).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	} else {
		mock.ExpectQuery(query).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"1"})) // empty = user not found
	}
}
