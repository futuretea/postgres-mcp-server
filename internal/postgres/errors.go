package postgres

import "fmt"

type safeError struct {
	code    string
	summary string
}

func (e safeError) Error() string {
	return fmt.Sprintf("[postgres_%s] %s", e.code, e.summary)
}

func postgresError(code, summary string) error {
	return safeError{code: code, summary: summary}
}

func connectionError() error {
	return postgresError("connection_failed", "database connection failed")
}

func metadataError() error {
	return postgresError("metadata_failed", "metadata query failed")
}

func queryError() error {
	return postgresError("query_failed", "query execution failed")
}
