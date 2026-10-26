package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var errModelTotalTimeout = errors.New("model total timeout")

type modelStreamStats struct {
	InputTokens, OutputTokens            int64
	AnswerCharacters, ThinkingCharacters int
	HTTPStatus                           int
	FinishReason                         string
}
type modelFailureError struct{ Kind, Message, ServiceCode string }

func (e *modelFailureError) Error() string { return e.Message }
func modelFailure(kind, message string) error {
	return &modelFailureError{Kind: kind, Message: message}
}

var errModelIdleTimeout = errors.New("model idle timeout")

type modelActivityReader struct {
	io.Reader
	activity func()
}

func (r modelActivityReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.activity()
	}
	return n, err
}
func modelStreamError(ctx context.Context, err error, idle, total time.Duration) error {
	if errors.Is(context.Cause(ctx), errModelIdleTimeout) {
		return modelFailure("idle_timeout", fmt.Sprintf("模型响应超时：连续 %s 未收到任何数据，已停止等待；这不是上下文超限。可在模型与设置中调整无响应超时，并检查模型服务及代理连接", idle))
	}
	if errors.Is(context.Cause(ctx), errModelTotalTimeout) {
		return modelFailure("total_timeout", fmt.Sprintf("模型达到设置的总超时 %s，已停止等待；可在模型与设置中调整总超时，0 表示不限制。这不是上下文超限", total))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return modelFailure("connection_timeout", "模型连接或读取超时，请检查模型服务及反向代理超时配置；已有候选未完成")
	}
	return modelFailure("connection_closed", "模型连接中断；已有候选未完成，可检查服务日志或代理连接")
}
func modelErrorDetail(raw []byte, key string) string {
	var message string
	if json.Unmarshal(raw, &message) != nil {
		var detail struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		if json.Unmarshal(raw, &detail) == nil {
			message = detail.Message
			if detail.Code != "" {
				message = detail.Code + "：" + message
			}
		}
	}
	if strings.TrimSpace(message) == "" {
		return "请检查模型服务日志"
	}
	if key != "" {
		message = strings.ReplaceAll(message, key, "[密钥已隐藏]")
	}
	runes := []rune(message)
	if len(runes) > 600 {
		message = string(runes[:600]) + "…"
	}
	return message
}
func modelHTTPError(res *http.Response, key string) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<10))
	var envelope struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &envelope) == nil {
		if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
			return modelServiceFailure(envelope.Error, key, fmt.Sprintf("模型返回 HTTP %d：", res.StatusCode))
		}
		if envelope.Message != "" {
			raw, _ = json.Marshal(envelope.Message)
			return modelFailure("service_error", fmt.Sprintf("模型返回 HTTP %d：%s", res.StatusCode, modelErrorDetail(raw, key)))
		}
	}
	return modelFailure("service_error", fmt.Sprintf("模型返回 HTTP %d，请检查模型名称、密钥、接口及服务端日志", res.StatusCode))
}

var safeModelCode = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`)

func modelServiceFailure(raw []byte, key, prefix string) error {
	var detail struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &detail)
	code := detail.Code
	if !safeModelCode.MatchString(code) || key != "" && strings.Contains(code, key) {
		code = ""
	}
	kind := "service_error"
	if code == "context_length_exceeded" || code == "max_context_length_exceeded" {
		kind = "service_context_limit"
	}
	return &modelFailureError{Kind: kind, Message: prefix + modelErrorDetail(raw, key), ServiceCode: code}
}
