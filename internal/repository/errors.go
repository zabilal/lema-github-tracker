package repository

import "github-service/pkg/errors"

var (
    // ErrNotFound is returned when a record is not found
    ErrNotFound = errors.New(
        errors.ErrorTypeNotFound,
        "record not found",
    )

    // ErrAlreadyExists is returned when a record already exists
    ErrAlreadyExists = errors.New(
        errors.ErrorTypeConflict,
        "record already exists",
    )

    // ErrOptimisticLock is returned when an optimistic lock fails
    ErrOptimisticLock = errors.New(
        errors.ErrorTypeConflict,
        "optimistic lock failed",
    )
)