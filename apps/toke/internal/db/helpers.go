package db

import (
	"context"
	"database/sql"
)

var globalDB *sql.DB
var globalQueries *Queries

// InitGlobalDB initializes the global database connection
func InitGlobalDB(ctx context.Context, dataDir string) error {
	db, err := Connect(ctx, dataDir)
	if err != nil {
		return err
	}
	globalDB = db
	globalQueries = New(db)
	return nil
}

// GetGlobalDB returns the global database connection
func GetGlobalDB() *sql.DB {
	return globalDB
}

// GetGlobalQueries returns the global queries instance
func GetGlobalQueries() *Queries {
	return globalQueries
}

// GetSession is a helper to get a session by ID
func GetSession(sessionID string) (*Session, error) {
	if globalQueries == nil {
		return nil, sql.ErrNoRows
	}
	session, err := globalQueries.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetSessionMessages is a helper to get all messages for a session
func GetSessionMessages(sessionID string) ([]Message, error) {
	if globalQueries == nil {
		return nil, sql.ErrNoRows
	}
	return globalQueries.ListMessagesBySession(context.Background(), sessionID)
}