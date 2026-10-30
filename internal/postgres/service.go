package postgres

import (
	"context"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
)

const (
	minimumServerVersion = 170000
	serverVersionSQL     = "SELECT current_setting('server_version_num')"
	serviceRoleSQL       = `SELECT rolsuper, rolbypassrls, rolreplication,
		pg_has_role(current_user, 'pg_read_server_files', 'member'),
		pg_has_role(current_user, 'pg_write_server_files', 'member'),
		pg_has_role(current_user, 'pg_execute_server_program', 'member'),
		pg_has_role(current_user, 'pg_signal_backend', 'member')
		FROM pg_roles WHERE rolname = current_user`
)

// Pool and Tx are private-API-sized seams for unit tests. The production
// adapter below remains the only owner of pgxpool resources.
type Pool interface {
	BeginTx(context.Context, pgx.TxOptions) (Tx, error)
	Close()
}

type Tx interface {
	QueryRow(context.Context, string, ...any) Row
	Rollback(context.Context) error
}

type queryTx interface {
	Tx
	Query(context.Context, string, ...any) (Rows, error)
}

type Row interface {
	Scan(...any) error
}

type Rows interface {
	FieldDescriptions() []pgconn.FieldDescription
	Next() bool
	Scan(...any) error
	Close()
	Err() error
}

// PoolFactory constructs a Service pool from a prevalidated execution config.
type PoolFactory func(context.Context, *config.PostgresExecutionConfig) (Pool, error)

// Service owns exactly one PostgreSQL pool and exposes only safe data values
// to the tool handlers.
type Service struct {
	pool      Pool
	closeOnce sync.Once
}

// NewService creates a pool then verifies server version and service-role
// safety before any tool can execute SQL. allowUnsafeDatabaseRole skips only
// the service-role rejection; the server version check always runs.
func NewService(ctx context.Context, executionConfig *config.PostgresExecutionConfig, factory PoolFactory, allowUnsafeDatabaseRole bool) (*Service, error) {
	if executionConfig == nil || executionConfig.PoolConfig == nil {
		return nil, connectionError()
	}
	if factory == nil {
		factory = newPGXPool
	}

	pool, err := factory(ctx, executionConfig)
	if err != nil || pool == nil {
		return nil, connectionError()
	}

	service := &Service{pool: pool}
	if err := service.verifyStartup(ctx, allowUnsafeDatabaseRole); err != nil {
		service.Close()
		return nil, err
	}
	return service, nil
}

func newPGXPool(ctx context.Context, executionConfig *config.PostgresExecutionConfig) (Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, executionConfig.PoolConfig)
	if err != nil {
		return nil, err
	}
	return pgxPool{Pool: pool}, nil
}

func (s *Service) verifyStartup(ctx context.Context, allowUnsafeDatabaseRole bool) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return connectionError()
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var versionText string
	if err := tx.QueryRow(ctx, serverVersionSQL, pgx.QueryExecModeExec).Scan(&versionText); err != nil {
		return connectionError()
	}
	version, err := strconv.Atoi(versionText)
	if err != nil {
		return connectionError()
	}
	if version < minimumServerVersion {
		return postgresError("unsupported_server_version", "PostgreSQL 17 or later is required")
	}

	var superuser, bypassRLS, replication bool
	var readFiles, writeFiles, executeProgram, signalBackend bool
	if err := tx.QueryRow(ctx, serviceRoleSQL, pgx.QueryExecModeExec).Scan(
		&superuser,
		&bypassRLS,
		&replication,
		&readFiles,
		&writeFiles,
		&executeProgram,
		&signalBackend,
	); err != nil {
		return connectionError()
	}
	if superuser || bypassRLS || replication || readFiles || writeFiles || executeProgram || signalBackend {
		if allowUnsafeDatabaseRole {
			log.Warn().Msg("database service role is not permitted; allow_unsafe_database_role is set, so the startup role check is skipped")
			return nil
		}
		return postgresError("unsafe_service_role", "database service role is not permitted")
	}
	return nil
}

// Close is idempotent because command lifecycles can unwind through more than
// one transport cleanup path.
func (s *Service) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.closeOnce.Do(s.pool.Close)
}

type pgxPool struct {
	*pgxpool.Pool
}

func (p pgxPool) BeginTx(ctx context.Context, options pgx.TxOptions) (Tx, error) {
	tx, err := p.Pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return pgxTx{Tx: tx}, nil
}

type pgxTx struct {
	pgx.Tx
}

func (t pgxTx) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return t.Tx.QueryRow(ctx, sql, args...)
}

func (t pgxTx) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := t.Tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxRows{Rows: rows}, nil
}

type pgxRows struct {
	pgx.Rows
}
