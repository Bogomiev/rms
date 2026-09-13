package postgres

import (
	"errors"

	"github.com/lib/pq"
)

const (
	ERR_ForeignKeyViolation                      = pq.ErrorCode("23503")
	ERR_UniqueViolationUpdate                    = pq.ErrorCode("23504")
	ERR_UniqueViolation                          = pq.ErrorCode("23505")
	ERR_InvalidTransactionState                  = pq.ErrorCode("25000")
	ERR_ReadOnlySQLTransaction                   = pq.ErrorCode("25006")
	ERR_SchemaAndDataStatementMixingNotSupported = pq.ErrorCode("25007")
	ERR_NoActiveSQLTransaction                   = pq.ErrorCode("25P01")
	ERR_InFailedSQLTransaction                   = pq.ErrorCode("25P02")
)

func isErrorPg(err error, code pq.ErrorCode) bool {
	var sqlErr *pq.Error

	if errors.As(err, &sqlErr) {
		if sqlErr.Code == code {
			return true
		}
	}

	return false
}
