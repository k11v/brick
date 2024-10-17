package postgresutil

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
)

func NewPool(ctx context.Context, log *slog.Logger, cfg *Config, development bool) (*pgxpool.Pool, error) {
	pgxCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, errors.Join(errors.New("failed to parse Postgres DSN"), err)
	}
	if development {
		pgxCfg.ConnConfig.Tracer = newTracer(log)
	}

	db, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create Postgres pool"), err)
	}

	if err = db.Ping(ctx); err != nil {
		log.Warn("failed to ping Postgres after creating pool", "error", err)
	}

	return db, nil
}

func newTracer(log *slog.Logger) *tracelog.TraceLog {
	loggerFunc := func(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]interface{}) {
		attrs := make([]slog.Attr, 0, len(data))
		for k, v := range data {
			attrs = append(attrs, slog.Any(k, v))
		}

		var lvl slog.Level
		switch level {
		case tracelog.LogLevelTrace:
			lvl = slog.LevelDebug
		case tracelog.LogLevelDebug:
			lvl = slog.LevelDebug
		case tracelog.LogLevelInfo:
			lvl = slog.LevelInfo
		case tracelog.LogLevelWarn:
			lvl = slog.LevelWarn
		case tracelog.LogLevelError:
			lvl = slog.LevelError
		default:
			lvl = slog.LevelError
		}

		log.LogAttrs(ctx, lvl, msg, attrs...)
	}
	return &tracelog.TraceLog{
		Logger:   tracelog.LoggerFunc(loggerFunc),
		LogLevel: tracelog.LogLevelDebug,
	}
}
