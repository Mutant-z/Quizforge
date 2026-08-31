package api

import "errors"

// AppError 业务错误，携带 HTTP 状态码与统一错误码。
type AppError struct {
	Status  int
	Code    ErrorCode
	Message string
	Detail  string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func NewError(status int, code ErrorCode, msg string) *AppError {
	return &AppError{Status: status, Code: code, Message: msg}
}

func WrapError(status int, code ErrorCode, msg string, err error) *AppError {
	return &AppError{Status: status, Code: code, Message: msg, Err: err}
}

func IsAppError(err error) bool {
	var ae *AppError
	return errors.As(err, &ae)
}

func AsAppError(err error) *AppError {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return nil
}

// NotFound 构造 404。
func NotFound(msg string) *AppError {
	return NewError(404, ErrNotFound, msg)
}

// InvalidRequest 构造 400。
func InvalidRequest(msg string) *AppError {
	return NewError(400, ErrInvalidRequest, msg)
}

// Unauthorized 构造 401。
func Unauthorized(msg string) *AppError {
	return NewError(401, ErrUnauthorized, msg)
}

// Conflict 构造 409。
func Conflict(msg string) *AppError {
	return NewError(409, ErrConflict, msg)
}

// Internal 构造 500。
func Internal(msg string, err error) *AppError {
	return WrapError(500, ErrInternal, msg, err)
}
