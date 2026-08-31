package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/observability"
)

// ErrorCode 统一错误码（docs/10）。
type ErrorCode string

const (
	ErrInvalidRequest   ErrorCode = "INVALID_REQUEST"
	ErrUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrNotFound         ErrorCode = "NOT_FOUND"
	ErrConflict         ErrorCode = "CONFLICT"
	ErrRateLimited      ErrorCode = "RATE_LIMITED"
	ErrModelError       ErrorCode = "MODEL_ERROR"
	ErrImportFailed     ErrorCode = "IMPORT_FAILED"
	ErrPipelineFailed   ErrorCode = "PIPELINE_FAILED"
	ErrValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrInternal         ErrorCode = "INTERNAL_ERROR"
)

// Response 统一响应结构：{data, error, request_id}
type Response struct {
	Data      interface{} `json:"data"`
	Error     *ErrorBody  `json:"error"`
	RequestID string      `json:"request_id"`
}

type ErrorBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Detail  string    `json:"detail,omitempty"`
}

// OK 输出成功响应。
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Data: data, Error: nil, RequestID: observability.RequestID(c.Request.Context())})
}

// Created 输出 201。
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{Data: data, Error: nil, RequestID: observability.RequestID(c.Request.Context())})
}

// Fail 输出错误响应。
func Fail(c *gin.Context, status int, code ErrorCode, msg string) {
	c.JSON(status, Response{Data: nil, Error: &ErrorBody{Code: code, Message: msg}, RequestID: observability.RequestID(c.Request.Context())})
}

// FailDetail 输出带 detail 的错误响应。
func FailDetail(c *gin.Context, status int, code ErrorCode, msg, detail string) {
	c.JSON(status, Response{Data: nil, Error: &ErrorBody{Code: code, Message: msg, Detail: detail}, RequestID: observability.RequestID(c.Request.Context())})
}

// Page 统一分页结构。
type Page struct {
	Items    interface{} `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// PageOK 输出分页响应。
func PageOK(c *gin.Context, items interface{}, total int64, page, pageSize int) {
	OK(c, Page{Items: items, Total: total, Page: page, PageSize: pageSize})
}
