package database

import (
	"database/sql"

	_ "github.com/lib/pq"
	"github.com/mager/occipital/config"
	"go.uber.org/zap"
)

// ProvideDatabase provides a postgres client
func ProvideDatabase(logger *zap.Logger, cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to open database connection", zap.Error(err))
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		logger.Error("Failed to ping database", zap.Error(err))
		return nil, err
	}

	logger.Info("Successfully connected to database", zap.String("database", cfg.DatabaseURL))
	return db, nil
}

var Options = ProvideDatabase
