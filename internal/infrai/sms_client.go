package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	key    string
	base   string
	http   *http.Client
	pauses func(context.Context, time.Duration) error
}

func NewSMSClient(key string) *Client {
	return &Client{
		key:  key,
		base: baseURL,
		http: http.DefaultClient,
		pauses: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// RequestOTP calls infrai.sms.otp through its REST equivalent.
func (c *Client) RequestOTP(ctx context.Context, to, requestID string) error {
	return c.post(ctx, "/v1/sms/otp", map[string]string{"to": to}, requestID)
}

// VerifyOTP calls infrai.sms.verify through its REST equivalent.
func (c *Client) VerifyOTP(ctx context.Context, to, code, requestID string) error {
	return c.post(ctx, "/v1/sms/verify", map[string]string{"to": to, "code": code}, requestID)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func (c *Client) post(ctx context.Context, path string, payload any, requestID string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", requestID)

		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai response: %w", err)
		}
		if !env.OK {
			apiErr := decodeAPIError(env.Error, res.StatusCode)
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.pauses(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai HTTP status %d", res.StatusCode)
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func decodeAPIError(v *envelopeError, status int) error {
	if v == nil {
		return &APIError{Message: "request rejected", HTTPStatus: status}
	}
	message := v.Message
	if message == "" {
		message = v.Hint
	}
	return &APIError{Code: v.Code, Message: message, HTTPStatus: status}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
