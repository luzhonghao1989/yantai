package workspace

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AIErrorRecord struct {
	ServiceCode           string `json:"serviceCode,omitempty"`
	ID                    string `json:"id"`
	CreatedAt             string `json:"createdAt"`
	Kind                  string `json:"kind"`
	Message               string `json:"message"`
	Model                 string `json:"model"`
	Provider              string `json:"provider"`
	Host                  string `json:"host"`
	Mode                  string `json:"mode"`
	ChapterID             string `json:"chapterId"`
	Revision              int64  `json:"revision"`
	DurationMs            int64  `json:"durationMs"`
	ContextLimit          int    `json:"contextLimit"`
	OutputLimit           int    `json:"outputLimit"`
	EstimatedInputTokens  int    `json:"estimatedInputTokens"`
	InputTokens           int64  `json:"inputTokens,omitempty"`
	OutputTokens          int64  `json:"outputTokens,omitempty"`
	AnswerCharacters      int    `json:"answerCharacters"`
	ThinkingCharacters    int    `json:"thinkingCharacters"`
	HTTPStatus            int    `json:"httpStatus,omitempty"`
	FinishReason          string `json:"finishReason,omitempty"`
	RequestTimeoutMinutes int    `json:"requestTimeoutMinutes"`
	IdleTimeoutMinutes    int    `json:"idleTimeoutMinutes"`
}

func newAIErrorRecord(cfg AIConfig, cp ContextPackage, mode string, start time.Time, stats modelStreamStats, err error) AIErrorRecord {
	kind := "generation_error"
	var failure *modelFailureError
	if errors.As(err, &failure) {
		kind = failure.Kind
	}
	if errors.Is(err, context.Canceled) {
		kind = "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		kind = "caller_timeout"
	}
	if !cp.Fits {
		kind = "context_limit"
	}
	summaries := map[string]string{"generation_error": "模型生成失败，请结合状态与服务端日志排查", "idle_timeout": "达到无响应超时，期间未收到任何数据", "total_timeout": "达到设置的请求总超时", "connection_timeout": "模型连接或读取超时", "connection_closed": "模型连接中断或提前结束，未收到完整结果", "output_limit": "达到本次输出上限，结果未完成（可能包含思考用量）", "content_filter": "模型内容过滤停止生成", "service_error": "模型服务拒绝请求，请核对 HTTP 状态与服务端日志", "cancelled": "请求已取消或客户端已断开", "caller_timeout": "调用方的等待期限已到", "context_limit": "预估输入超过当前设置的可用输入预算", "service_context_limit": "模型服务报告上下文超限，请核对服务端实际加载窗口", "invalid_stream": "模型返回了无法解析的流式响应", "empty_answer": "模型只返回思考或未提供正文", "local_output_limit": "收到的输出超过本地保护容量"}
	host := ""
	if parsed, e := url.Parse(cfg.BaseURL); e == nil {
		host = parsed.Host
	}
	finish := stats.FinishReason
	switch finish {
	case "", "stop", "length", "max_tokens", "content_filter", "tool_calls", "function_call":
	default:
		finish = "other"
	}
	// Use fixed diagnostic summaries: provider error bodies can echo prompts or keys.
	model := cfg.Model
	if cfg.APIKey != "" {
		model = strings.ReplaceAll(model, cfg.APIKey, "[密钥已隐藏]")
		host = strings.ReplaceAll(host, cfg.APIKey, "[密钥已隐藏]")
	}
	serviceCode := ""
	if failure != nil {
		serviceCode = failure.ServiceCode
	}
	return AIErrorRecord{ServiceCode: serviceCode, ID: ID(), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Kind: kind, Message: summaries[kind], Model: model, Provider: cfg.Provider, Host: host, Mode: mode, ChapterID: cp.ChapterID, Revision: cp.Revision, DurationMs: time.Since(start).Milliseconds(), ContextLimit: cfg.ContextLimit, OutputLimit: cfg.OutputLimit, EstimatedInputTokens: cp.EstimatedTokens, InputTokens: stats.InputTokens, OutputTokens: stats.OutputTokens, AnswerCharacters: stats.AnswerCharacters, ThinkingCharacters: stats.ThinkingCharacters, HTTPStatus: stats.HTTPStatus, FinishReason: finish, RequestTimeoutMinutes: cfg.RequestTimeoutMinutes, IdleTimeoutMinutes: cfg.IdleTimeoutMinutes}
}

const aiErrorLogLimit = 1 << 20

func (s *Store) recordAIError(record AIErrorRecord) error {
	s.aiLogMu.Lock()
	defer s.aiLogMu.Unlock()
	dir := filepath.Join(s.Root, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "ai-errors.jsonl")
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if info, e := os.Stat(path); e == nil && info.Size()+int64(len(raw)) > aiErrorLogLimit {
		if e = os.Remove(path + ".3"); e != nil && !os.IsNotExist(e) {
			return e
		}
		for i := 2; i >= 0; i-- {
			src := path
			if i > 0 {
				src += "." + strconv.Itoa(i)
			}
			if e = os.Rename(src, path+"."+strconv.Itoa(i+1)); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(raw)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (s *Store) aiErrors() ([]AIErrorRecord, error) {
	s.aiLogMu.Lock()
	defer s.aiLogMu.Unlock()
	records := []AIErrorRecord{}
	for i := 3; i >= 0; i-- {
		path := filepath.Join(s.Root, "logs", "ai-errors.jsonl")
		if i > 0 {
			path += "." + strconv.Itoa(i)
		}
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(io.LimitReader(file, aiErrorLogLimit+16384))
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			var record AIErrorRecord
			if json.Unmarshal(scanner.Bytes(), &record) == nil {
				records = append(records, record)
				if len(records) > 100 {
					records = records[1:]
				}
			}
		}
		err = scanner.Err()
		file.Close()
		if err != nil {
			return nil, err
		}
	}
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}
func (s *Server) registerAIErrorLog(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ai/errors", func(w http.ResponseWriter, r *http.Request) {
		records, err := s.Store.aiErrors()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, records)
	})
}
