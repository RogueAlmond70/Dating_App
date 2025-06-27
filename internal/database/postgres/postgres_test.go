package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/RogueAlmond70/Muzz/explorepb"
	"github.com/RogueAlmond70/Muzz/internal/database/postgres"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestPutDecision_MutualLike(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	ctx := context.Background()
	req := &explorepb.PutDecisionRequest{
		ActorUserId:     "user1",
		RecipientUserId: "user2",
		LikedRecipient:  true,
	}

	// Expect INSERT/UPDATE
	mock.ExpectExec(`INSERT INTO likes`).WithArgs("user1", "user2", true).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect check for mutual like
	mock.ExpectQuery(`SELECT TRUE FROM likes`).
		WithArgs("user1", "user2").
		WillReturnRows(sqlmock.NewRows([]string{"true"}).AddRow(true))

	resp, err := p.PutDecision(ctx, req)
	assert.NoError(t, err)
	assert.True(t, resp.MutualLikes)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPutDecision_NoMutualLike(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	ctx := context.Background()
	req := &explorepb.PutDecisionRequest{
		ActorUserId:     "user1",
		RecipientUserId: "user2",
		LikedRecipient:  true,
	}

	mock.ExpectExec(`INSERT INTO likes`).WithArgs("user1", "user2", true).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(`SELECT TRUE FROM likes`).
		WithArgs("user1", "user2").
		WillReturnError(sql.ErrNoRows)

	resp, err := p.PutDecision(ctx, req)
	assert.NoError(t, err)
	assert.False(t, resp.MutualLikes)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetCountLikedYou(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	req := &explorepb.CountLikedYouRequest{RecipientUserId: "user123"}
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT l\.liker_id\)`).
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	resp, err := p.GetCountLikedYou(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), resp.Count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCursorEncodeDecode(t *testing.T) {
	id := 42
	createdAt := uint64(time.Now().Unix())

	token, err := postgres.EncodeCursor(id, createdAt)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	cursor, err := postgres.DecodeCursor(token)
	assert.NoError(t, err)
	assert.Equal(t, id, cursor.ID)
	assert.Equal(t, createdAt, cursor.CreatedAt)
}

func TestGetLikes_WithPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	ctx := context.Background()

	cursorTime := time.Now().Add(-1 * time.Hour)
	cursorID := 999
	token, err := postgres.EncodeCursor(cursorID, uint64(cursorTime.Unix()))
	assert.NoError(t, err)

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user123",
		PaginationToken: &token,
	}

	// Prepare rows: 26 rows = 25 + 1 for pagination token generation
	rows := sqlmock.NewRows([]string{"liker_id", "created_at", "id"})
	for i := 0; i < 26; i++ {
		rows.AddRow(fmt.Sprintf("liker%d", i), time.Now().Add(-time.Duration(i)*time.Minute), 1000+i)
	}

	mock.ExpectQuery(`WITH latest_likes AS .*SELECT liker_id, created_at, id.*`).
		WithArgs("user123", cursorTime.UTC(), cursorID, 26).
		WillReturnRows(rows)

	resp, err := p.GetLikes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Likers, 25)
	assert.NotNil(t, resp.NextPaginationToken)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLikes_NoPaginationToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	ctx := context.Background()

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user456",
		PaginationToken: nil,
	}

	rows := sqlmock.NewRows([]string{"liker_id", "created_at", "id"})
	for i := 0; i < 10; i++ {
		rows.AddRow(fmt.Sprintf("liker%d", i), time.Now().Add(-time.Duration(i)*time.Minute), 2000+i)
	}

	mock.ExpectQuery(`WITH latest_likes AS .*SELECT liker_id, created_at, id.*`).
		WithArgs("user456", 26).
		WillReturnRows(rows)

	resp, err := p.GetLikes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Likers, 10)
	assert.Nil(t, resp.NextPaginationToken)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNewLikes_WithPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	ctx := context.Background()
	cursorTime := time.Now().Add(-1 * time.Hour)
	cursorID := 777
	token, err := postgres.EncodeCursor(cursorID, uint64(cursorTime.Unix()))
	assert.NoError(t, err)

	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user789",
		PaginationToken: &token,
	}

	rows := sqlmock.NewRows([]string{"liker_id", "created_at", "id"})
	for i := 0; i < 26; i++ {
		rows.AddRow(fmt.Sprintf("newliker%d", i), time.Now().Add(-time.Duration(i)*time.Minute), 3000+i)
	}

	mock.ExpectQuery(`SELECT l.liker_id, l.created_at, l.id FROM likes l .*`).
		WithArgs("user789", cursorTime.UTC(), cursorID, 26).
		WillReturnRows(rows)

	resp, err := p.GetNewLikes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Likers, 25)
	assert.NotNil(t, resp.NextPaginationToken)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNewLikes_NoPaginationToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	logger := zaptest.NewLogger(t)
	p := postgres.NewPostgres(*logger, db)

	ctx := context.Background()
	req := &explorepb.ListLikedYouRequest{
		RecipientUserId: "user999",
		PaginationToken: nil,
	}

	rows := sqlmock.NewRows([]string{"liker_id", "created_at", "id"})
	for i := 0; i < 10; i++ {
		rows.AddRow(fmt.Sprintf("freshliker%d", i), time.Now().Add(-time.Duration(i)*time.Minute), 4000+i)
	}

	mock.ExpectQuery(`SELECT l.liker_id, l.created_at, l.id FROM likes l .*`).
		WithArgs("user999", 26).
		WillReturnRows(rows)

	resp, err := p.GetNewLikes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Likers, 10)
	assert.Nil(t, resp.NextPaginationToken)
	assert.NoError(t, mock.ExpectationsWereMet())
}
