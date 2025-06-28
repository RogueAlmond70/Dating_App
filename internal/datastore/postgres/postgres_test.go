package postgres_test

import (
	"context"
	"database/sql"

	//"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/datastore/postgres"

	//"github.com/RogueAlmond70/Muzz/internal/handler"
	"go.uber.org/zap"
	//"google.golang.org/grpc/codes"
	//"google.golang.org/grpc/status"
)

func newPostgresWithMock(t *testing.T) (*postgres.Postgres, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	logger := zap.NewNop()
	pg := postgres.NewPostgres(logger, db)
	return pg, mock, func() {
		db.Close()
	}
}

func TestGetLikes_NoPaginationToken_Success(t *testing.T) {
	pg, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user1",
	}

	// Prepare expected SQL query & rows
	queryRegex := regexp.QuoteMeta(`
	WITH latest_likes AS (
	SELECT DISTINCT ON (liker_id) liker_id, created_at, id
	FROM likes
	WHERE liked_id = $1 AND decision = true
	AND created_at >= NOW() - INTERVAL '1 year'
	ORDER BY liker_id, created_at DESC, id DESC
	)
	SELECT liker_id, created_at, id
	FROM latest_likes 
	 ORDER BY created_at DESC, id DESC LIMIT $2`)

	columns := []string{"liker_id", "created_at", "id"}
	now := time.Now()
	rows := sqlmock.NewRows(columns).
		AddRow("likerA", now, 1).
		AddRow("likerB", now.Add(-time.Minute), 2)

	mock.ExpectQuery(queryRegex).
		WithArgs(req.RecipientUserId, 26).
		WillReturnRows(rows)

	resp, err := pg.GetLikes(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resp.Likers) != 2 {
		t.Errorf("expected 2 likers, got %d", len(resp.Likers))
	}

	if resp.NextPaginationToken == nil || *resp.NextPaginationToken != "" {
		// Since only 2 results and pageSize is 25, no next token expected
		t.Logf("next token: %v", resp.NextPaginationToken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestPutDecision_InsertAndMutualLike(t *testing.T) {
	pg, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()

	req := &explorepb.PutDecisionRequest{
		ActorUserId:     "actor1",
		RecipientUserId: "user2",
		LikedRecipient:  true,
	}

	// Expect insert or update exec
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (liker_id, liked_id)
			DO UPDATE SET decision = EXCLUDED.decision, decision_at = EXCLUDED.decision_at
			RETURNING decision
			`)).
		WithArgs(req.ActorUserId, req.RecipientUserId, req.LikedRecipient).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect query for mutual like check returns TRUE
	rows := sqlmock.NewRows([]string{"true"}).AddRow(true)
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT TRUE
			FROM likes
			WHERE liker_id = $1 AND liked_id = $2 AND decision
			LIMIT 1
		`)).
		WithArgs(req.ActorUserId, req.RecipientUserId).
		WillReturnRows(rows)

	resp, err := pg.PutDecision(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.MutualLikes {
		t.Errorf("expected mutual likes true, got false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestPutDecision_NoMutualLike(t *testing.T) {
	pg, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()

	req := &explorepb.PutDecisionRequest{
		ActorUserId:     "actor1",
		RecipientUserId: "user2",
		LikedRecipient:  true,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO likes (liker_id, liked_id, decision, decision_at, created_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (liker_id, liked_id)
			DO UPDATE SET decision = EXCLUDED.decision, decision_at = EXCLUDED.decision_at
			RETURNING decision
			`)).
		WithArgs(req.ActorUserId, req.RecipientUserId, req.LikedRecipient).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// QueryRow returns sql.ErrNoRows to simulate no mutual like
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT TRUE
			FROM likes
			WHERE liker_id = $1 AND liked_id = $2 AND decision
			LIMIT 1
		`)).
		WithArgs(req.ActorUserId, req.RecipientUserId).
		WillReturnError(sql.ErrNoRows)

	resp, err := pg.PutDecision(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.MutualLikes {
		t.Errorf("expected mutual likes false, got true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetCountLikedYou_Success(t *testing.T) {
	pg, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()

	req := &explorepb.CountLikedYouRequest{
		RecipientUserId: "user1",
	}

	count := int64(42)
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
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))

	resp, err := pg.GetCountLikedYou(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Count != uint64(count) {
		t.Errorf("expected count %d, got %d", count, resp.Count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
