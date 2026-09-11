package domain

import (
	"context"
	"errors"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }
func Input(message string) *Error {
	return &Error{Status: 400, Code: "INVALID_INPUT", Message: message}
}
func Missing() *Error {
	return &Error{Status: 404, Code: "NOT_FOUND", Message: "대상을 찾을 수 없습니다."}
}
func Conflict(code, message string) *Error { return &Error{Status: 409, Code: code, Message: message} }

// PublicError is the only path from internal errors to HTTP/CLI diagnostics.
// It never includes a query, connection string or the underlying driver's message.
func PublicError(err error) *Error {
	var public *Error
	if errors.As(err, &public) {
		return public
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Missing()
	}
	var database *pgconn.PgError
	if errors.As(err, &database) {
		switch database.Code {
		case "23505":
			return Conflict("DUPLICATE", "이미 존재하는 코드 또는 기록입니다.")
		case "40P01", "40001", "55P03", "57014", "57P01", "57P02", "57P03", "53300", "08000", "08003", "08006":
			return Unavailable()
		}
	}
	var network net.Error
	var connection *pgconn.ConnectError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &network) || errors.As(err, &connection) || pgconn.SafeToRetry(err) {
		return Unavailable()
	}
	return &Error{Status: 500, Code: "INTERNAL_ERROR", Message: "요청을 처리하지 못했습니다. 같은 요청 키로 결과를 다시 확인해 주세요."}
}

func Unavailable() *Error {
	return &Error{Status: 503, Code: "DB_UNAVAILABLE", Message: "데이터베이스 연결 또는 잠금 대기로 요청을 완료하지 못했습니다. 같은 요청으로 다시 확인해 주세요."}
}
