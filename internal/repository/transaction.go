package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github-service/pkg/logger"
	"time"

	"github.com/pkg/errors"
)

// Transaction represents a database transaction with additional metadata
type Transaction struct {
	tx        *sql.Tx
	db        *sql.DB
	committed bool
	rolledBack bool
	logger    logger.Logger
	startTime time.Time
	ctx       context.Context
	name      string
}

// TransactionManager handles database transactions with proper lifecycle management
type TransactionManager interface {
	// WithTransaction executes a function within a database transaction
	WithTransaction(ctx context.Context, name string, fn func(tx Transaction) error) error

	// WithTransactionTimeout executes a function within a transaction with timeout
	WithTransactionTimeout(ctx context.Context, name string, timeout time.Duration, fn func(tx Transaction) error) error

	// BeginTransaction starts a new transaction manually (use with caution)
	BeginTransaction(ctx context.Context, name string) (Transaction, error)
}

// transactionManager implements TransactionManager interface
type transactionManager struct {
	db     *sql.DB
	logger logger.Logger
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(db *sql.DB, logger logger.Logger) TransactionManager {
	return &transactionManager{
		db:     db,
		logger: logger,
	}
}

// WithTransaction executes a function within a database transaction
func (tm *transactionManager) WithTransaction(ctx context.Context, name string, fn func(tx Transaction) error) error {
	return tm.WithTransactionTimeout(ctx, name, 30*time.Second, fn)
}

// WithTransactionTimeout executes a function within a transaction with timeout
func (tm *transactionManager) WithTransactionTimeout(ctx context.Context, name string, timeout time.Duration, fn func(tx Transaction) error) error {
	// Create context with timeout
	txCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Begin transaction with proper isolation level
	tx, err := tm.db.BeginTx(txCtx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
		ReadOnly:  false,
	})
	if err != nil {
		tm.logger.Error("failed to begin transaction",
			"transaction_name", name,
			"error", err)
		return errors.Wrap(err, "failed to begin transaction")
	}

	transaction := Transaction{
		tx:        tx,
		db:        tm.db,
		logger:    tm.logger,
		startTime: time.Now(),
		ctx:       txCtx,
		name:      name,
	}

	// Ensure transaction is always closed
	defer func() {
		if err := transaction.close(); err != nil {
			tm.logger.Error("failed to close transaction",
				"transaction_name", name,
				"error", err)
		}
	}()

	tm.logger.Info("transaction started",
		"transaction_name", name,
		"timeout", timeout)

	// Execute the function within transaction
	if err := fn(transaction); err != nil {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil {
			tm.logger.Error("failed to rollback transaction",
				"transaction_name", name,
				"original_error", err,
				"rollback_error", rollbackErr)
			return errors.Wrap(err, "transaction failed and rollback failed")
		}

		tm.logger.Error("transaction rolled back",
			"transaction_name", name,
			"duration", time.Since(transaction.startTime),
			"error", err)
		return errors.Wrap(err, "transaction rolled back")
	}

	// Commit the transaction
	if err := transaction.Commit(); err != nil {
		tm.logger.Error("failed to commit transaction",
			"transaction_name", name,
			"duration", time.Since(transaction.startTime),
			"error", err)
		return errors.Wrap(err, "failed to commit transaction")
	}

	tm.logger.Info("transaction committed successfully",
		"transaction_name", name,
		"duration", time.Since(transaction.startTime))

	return nil
}

// BeginTransaction starts a new transaction manually
func (tm *transactionManager) BeginTransaction(ctx context.Context, name string) (Transaction, error) {
	tx, err := tm.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
		ReadOnly:  false,
	})
	if err != nil {
		return Transaction{}, errors.Wrap(err, "failed to begin transaction")
	}

	return Transaction{
		tx:        tx,
		db:        tm.db,
		logger:    tm.logger,
		startTime: time.Now(),
		ctx:       ctx,
		name:      name,
	}, nil
}

// Transaction methods

// Exec executes a query that doesn't return rows
func (t *Transaction) Exec(query string, args ...interface{}) (sql.Result, error) {
	t.logger.Debug("executing transaction query",
		"transaction_name", t.name,
		"query", query)

	result, err := t.tx.ExecContext(t.ctx, query, args...)
	if err != nil {
		t.logger.Error("transaction query failed",
			"transaction_name", t.name,
			"query", query,
			"error", err)
		return nil, errors.Wrap(err, "transaction exec failed")
	}

	return result, nil
}

// Query executes a query that returns rows
func (t *Transaction) Query(query string, args ...interface{}) (*sql.Rows, error) {
	t.logger.Debug("executing transaction query",
		"transaction_name", t.name,
		"query", query)

	rows, err := t.tx.QueryContext(t.ctx, query, args...)
	if err != nil {
		t.logger.Error("transaction query failed",
			"transaction_name", t.name,
			"query", query,
			"error", err)
		return nil, errors.Wrap(err, "transaction query failed")
	}

	return rows, nil
}

// QueryRow executes a query that returns at most one row
func (t *Transaction) QueryRow(query string, args ...interface{}) *sql.Row {
	t.logger.Debug("executing transaction query row",
		"transaction_name", t.name,
		"query", query)

	return t.tx.QueryRowContext(t.ctx, query, args...)
}

// Prepare creates a prepared statement for later queries or executions
func (t *Transaction) Prepare(query string) (*sql.Stmt, error) {
	stmt, err := t.tx.PrepareContext(t.ctx, query)
	if err != nil {
		t.logger.Error("failed to prepare statement",
			"transaction_name", t.name,
			"query", query,
			"error", err)
		return nil, errors.Wrap(err, "failed to prepare statement")
	}

	return stmt, nil
}

// Commit commits the transaction
func (t *Transaction) Commit() error {
    if t.tx == nil {
        return errors.New("no transaction to commit")
    }
    
    if t.rolledBack {
        return errors.New("cannot commit: transaction was rolled back")
    }
    
    if t.committed {
        return errors.New("transaction already committed")
    }
    
    err := t.tx.Commit()
    if err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }
    
    t.committed = true
    t.tx = nil
    return nil
}

// Rollback rolls back the transaction
func (t *Transaction) Rollback() error {
    if t.tx == nil {
        return nil // No transaction to roll back
    }
    
    if t.committed {
        return nil // Already committed, nothing to roll back
    }
    
    if t.rolledBack {
        return nil // Already rolled back
    }
    
    err := t.tx.Rollback()
    if err != nil && err != sql.ErrTxDone {
        return fmt.Errorf("failed to rollback transaction: %w", err)
    }
    
    t.rolledBack = true
    t.tx = nil
    return nil
}
// Context returns the transaction context
func (t *Transaction) Context() context.Context {
	return t.ctx
}

// close ensures the transaction is properly closed
func (t *Transaction) close() error {
	if t.tx == nil {
		return nil // Already closed
	}

	// If we reach here, transaction wasn't committed, so rollback
	return t.Rollback()
}

// Repository interfaces that support transactions

// RepositoryTransactionRepo defines repository operations that can be performed within a transaction
// type RepositoryTransactionRepo interface {
// 	// CreateRepository creates a new repository record within a transaction
// 	CreateRepository(tx Transaction, repo *Repository) error

// 	// UpdateRepository updates repository metadata within a transaction
// 	UpdateRepository(tx Transaction, repo *Repository) error

// 	// UpdateRepositorySyncStatus updates sync status within a transaction
// 	UpdateRepositorySyncStatus(tx Transaction, id int64, status models.SyncStatus, syncedAt time.Time) error

// 	// GetRepositoryForUpdate gets repository with row lock for updates
// 	GetRepositoryForUpdate(tx Transaction, owner, name string) (*Repository, error)
// }

// CommitTransactionRepo defines commit operations that can be performed within a transaction
// type CommitTransactionRepo interface {
// 	// BatchInsertCommits inserts multiple commits within a transaction
// 	BatchInsertCommits(tx Transaction, commits []models.Commit) error

// 	// DeleteCommitsAfterDate deletes commits after a specific date within a transaction
// 	DeleteCommitsAfterDate(tx Transaction, repositoryID int64, date time.Time) error

// 	// UpdateCommitStats updates commit statistics within a transaction
// 	UpdateCommitStats(tx Transaction, repositoryID int64, stats models.CommitStats) error
// }
