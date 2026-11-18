// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pulumi/pulumi-go-provider/infer"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	// agentsBetaHeader opts in to the beta Agents API.
	agentsBetaHeader = "agents=v1"

	// The environment variables that the provider configuration falls back to.
	apiTokenEnv = "OPENAI_API_KEY"
	baseURLEnv  = "OPENAI_BASE_URL"
)

// client is a minimal client for the OpenAI REST API.
type client struct {
	baseURL string
	token   string
	http    *http.Client
}

// newClient builds a client from the provider configuration.
//
// The environment variables are read here as well as being schema defaults. The engine
// applies schema defaults only when it checks the provider's configuration, and it does
// not always do that: a default provider that `pulumi import` recorded in the state is
// configured from its stored, unchecked inputs on the next import.
func newClient(ctx context.Context) (*client, error) {
	cfg := infer.GetConfig[Config](ctx)
	token := cfg.ApiToken
	if token == "" {
		token = os.Getenv(apiTokenEnv)
	}
	if token == "" {
		return nil, errors.New("no OpenAI API token configured: " +
			"set `openai:apiToken` or the " + apiTokenEnv + " environment variable")
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv(baseURLEnv)
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

// apiError is an error response from the OpenAI API.
type apiError struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
}

func (e *apiError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	var detail []string
	if e.Type != "" {
		detail = append(detail, "type: "+e.Type)
	}
	if e.Code != "" {
		detail = append(detail, "code: "+e.Code)
	}
	if len(detail) > 0 {
		msg += " (" + strings.Join(detail, ", ") + ")"
	}
	return fmt.Sprintf("OpenAI API error %d: %s", e.StatusCode, msg)
}

// isNotFound reports whether err is an API error with a 404 status.
func isNotFound(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// isConflict reports whether err is an API error with a 409 status.
func isConflict(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

// How conflicts are retried: the first wait, which doubles each time, and the number of
// attempts.
const (
	conflictRetryDelay = 200 * time.Millisecond
	conflictAttempts   = 7
)

// retryConflicts calls fn until it returns anything but a conflict, waiting a little
// longer each time, for up to about 12 seconds.
func retryConflicts(ctx context.Context, fn func() error) error {
	delay := conflictRetryDelay
	for attempt := 1; ; attempt++ {
		err := fn()
		if !isConflict(err) || attempt == conflictAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// do sends a request with an optional JSON body and returns the raw response body.
func (c *client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("OpenAI-Beta", agentsBetaHeader)
	req.Header.Set("User-Agent", "pulumi-openai/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck // Nothing useful to do with a close error.

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading response: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{StatusCode: resp.StatusCode}
		var envelope struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    any    `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &envelope) == nil {
			apiErr.Message = envelope.Error.Message
			apiErr.Type = envelope.Error.Type
			if envelope.Error.Code != nil {
				apiErr.Code = fmt.Sprint(envelope.Error.Code)
			}
		}
		return nil, apiErr
	}
	return respBody, nil
}
