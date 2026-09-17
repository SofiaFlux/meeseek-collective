package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/identity"
)

type API interface {
	Status(context.Context) (StatusDTO, error)
	CreateTask(context.Context, CreateTaskRequest) (TaskDTO, error)
	Task(context.Context, domain.ID) (TaskDTO, error)
	Approve(context.Context, domain.ID, identity.Signer) (ApprovalDTO, error)
	Attempt(context.Context, domain.ID) (AttemptDTO, error)
	Operation(context.Context, domain.ID) (OperationDTO, error)
	Shutdown(context.Context) error
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, client *http.Client) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://meeseek.local"
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{baseURL: baseURL, token: strings.TrimSpace(token), http: client}
}

func NewLocalClient(endpoint, token string) *Client {
	return NewClient("http://meeseek.local", token, localHTTPClient(endpoint))
}

func (c *Client) Status(ctx context.Context) (StatusDTO, error) {
	var result StatusDTO
	err := c.doJSON(ctx, http.MethodGet, "/status", nil, &result, http.StatusOK)
	return result, err
}

func (c *Client) CreateTask(ctx context.Context, request CreateTaskRequest) (TaskDTO, error) {
	var result TaskDTO
	err := c.doJSON(ctx, http.MethodPost, "/tasks", request, &result, http.StatusCreated)
	return result, err
}

func (c *Client) Task(ctx context.Context, id domain.ID) (TaskDTO, error) {
	var result TaskDTO
	err := c.doJSON(ctx, http.MethodGet, "/tasks/"+string(id), nil, &result, http.StatusOK)
	return result, err
}

func (c *Client) Approve(ctx context.Context, id domain.ID, signer identity.Signer) (ApprovalDTO, error) {
	if signer == nil {
		return ApprovalDTO{}, errors.New("Owner signer is required")
	}
	var challenge ApprovalChallengeDTO
	if err := c.doJSON(ctx, http.MethodGet, "/approvals/"+string(id)+"/challenge", nil, &challenge, http.StatusOK); err != nil {
		return ApprovalDTO{}, err
	}
	signature, err := signer.Sign(ApprovalSigningMessage(challenge.Challenge, challenge.RequestDigest))
	if err != nil {
		return ApprovalDTO{}, fmt.Errorf("sign approval challenge: %w", err)
	}
	request := ApprovalRequest{Challenge: challenge.Challenge, Signature: base64.StdEncoding.EncodeToString(signature)}
	var result ApprovalDTO
	err = c.doJSON(ctx, http.MethodPost, "/approvals/"+string(id), request, &result, http.StatusOK)
	return result, err
}

func (c *Client) Attempt(ctx context.Context, id domain.ID) (AttemptDTO, error) {
	var result AttemptDTO
	err := c.doJSON(ctx, http.MethodGet, "/inspect/attempts/"+string(id), nil, &result, http.StatusOK)
	return result, err
}

func (c *Client) Operation(ctx context.Context, id domain.ID) (OperationDTO, error) {
	var result OperationDTO
	err := c.doJSON(ctx, http.MethodGet, "/inspect/operations/"+string(id), nil, &result, http.StatusOK)
	return result, err
}

func (c *Client) Shutdown(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/shutdown", nil, nil, http.StatusAccepted)
}

func (c *Client) doJSON(ctx context.Context, method, path string, requestBody, responseBody any, wantStatus int) error {
	if c == nil || c.http == nil || strings.TrimSpace(c.baseURL) == "" {
		return errors.New("control client is not configured")
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode control request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("control request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		var apiError struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&apiError)
		if strings.TrimSpace(apiError.Error) == "" {
			apiError.Error = response.Status
		}
		return fmt.Errorf("control API %s %s: %s", method, path, apiError.Error)
	}
	if responseBody == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(responseBody); err != nil {
		return fmt.Errorf("decode control response: %w", err)
	}
	return nil
}
