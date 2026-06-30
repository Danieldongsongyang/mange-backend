package dreamina

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/dreamina/authsign"
)

const (
	DefaultBaseURL     = "https://jimeng.jianying.com"
	DefaultAID         = "513695"
	DefaultAgentDetect = "agent:codex"
	DefaultFrom        = "dreamina_cli"
	DefaultCLIVersion  = "2a20fff-dirty"
	DefaultPF          = "7"

	HeaderAppID   = "appid"
	HeaderPF      = "pf"
	HeaderTTLogID = "X-TT-LOGID"

	EndpointUserInfo      = "/dreamina/cli/v1/dreamina_cli_user_info"
	EndpointImageGenerate = "/dreamina/cli/v1/image_generate"
	EndpointHistoryByIDs  = "/mweb/v1/get_history_by_ids"
	EndpointUploadToken   = "/mweb/v1/get_upload_token"
	EndpointResourceStore = "/dreamina/mcp/v1/resource_store"
)

var ErrInvalidClientConfig = errors.New("invalid dreamina client config")

type Client struct {
	httpClient *http.Client
	provider   CredentialProvider
	baseURL    string

	aid         string
	cliVersion  string
	agentDetect string
	from        string
	userAgent   string

	signOptions authsign.SignOptions
}

type ClientOption func(*Client)

func NewClient(provider CredentialProvider, opts ...ClientOption) (*Client, error) {
	c := &Client{
		httpClient:  http.DefaultClient,
		provider:    provider,
		baseURL:     DefaultBaseURL,
		aid:         DefaultAID,
		cliVersion:  DefaultCLIVersion,
		agentDetect: DefaultAgentDetect,
		from:        DefaultFrom,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.provider == nil {
		return nil, fmt.Errorf("%w: credential provider is required", ErrInvalidClientConfig)
	}
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}
	c.baseURL = strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	if c.baseURL == "" {
		return nil, fmt.Errorf("%w: base url is required", ErrInvalidClientConfig)
	}
	if _, err := url.ParseRequestURI(c.baseURL); err != nil {
		return nil, fmt.Errorf("%w: invalid base url", ErrInvalidClientConfig)
	}
	return c, nil
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

func WithCLIVersion(cliVersion string) ClientOption {
	return func(c *Client) {
		c.cliVersion = cliVersion
	}
}

func WithAID(aid string) ClientOption {
	return func(c *Client) {
		c.aid = aid
	}
}

func WithAgentDetect(agentDetect string) ClientOption {
	return func(c *Client) {
		c.agentDetect = agentDetect
	}
}

func WithFrom(from string) ClientOption {
	return func(c *Client) {
		c.from = from
	}
}

func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

func WithSignOptions(opts authsign.SignOptions) ClientOption {
	return func(c *Client) {
		c.signOptions = opts
	}
}

func (c *Client) UserInfo(ctx context.Context) (*UserInfoResponse, error) {
	var out UserInfoResponse
	if err := c.doJSON(ctx, http.MethodGet, EndpointUserInfo, nil, nil, &out); err != nil {
		return nil, err
	}
	if err := out.Check(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetUploadToken(ctx context.Context, scene int) (*UploadTokenResponse, error) {
	if scene == 0 {
		scene = 2
	}
	payload := uploadTokenRequest{Scene: scene}
	var out UploadTokenResponse
	if err := c.doJSON(ctx, http.MethodPost, EndpointUploadToken, nil, payload, &out); err != nil {
		return nil, err
	}
	if err := out.Check(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SubmitTextToImage(ctx context.Context, input TextToImageRequest) (*ImageGenerateResponse, error) {
	payload, query, err := buildImageGeneratePayload(input)
	if err != nil {
		return nil, err
	}
	var out ImageGenerateResponse
	if err := c.doJSON(ctx, http.MethodPost, EndpointImageGenerate, query, payload, &out); err != nil {
		return nil, err
	}
	if err := out.Check(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) FetchHistory(ctx context.Context, submitIDs []string) (*HistoryResponse, error) {
	ids := make([]string, 0, len(submitIDs))
	for _, id := range submitIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: submit ids are required", ErrInvalidRequest)
	}

	payload := historyByIDsRequest{
		HistoryIDs: nil,
		NeedBatch:  true,
		SubmitIDs:  ids,
	}
	var out HistoryResponse
	if err := c.doJSON(ctx, http.MethodPost, EndpointHistoryByIDs, nil, payload, &out); err != nil {
		return nil, err
	}
	if err := out.Check(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	req, err := c.newJSONRequest(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	if err := c.authorize(req); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dreamina request failed: method=%s path=%s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read dreamina response failed: method=%s path=%s: %w", method, path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &HTTPStatusError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
		}
	}
	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	if err := common.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode dreamina response failed: method=%s path=%s: %w", method, path, err)
	}
	return nil
}

func (c *Client) newJSONRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("%w: path must start with /", ErrInvalidRequest)
	}

	requestURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid request url", ErrInvalidRequest)
	}
	values := c.defaultQuery()
	for key, vals := range query {
		for _, val := range vals {
			values.Add(key, val)
		}
	}
	requestURL.RawQuery = values.Encode()

	var reader io.Reader
	if body != nil {
		data, err := common.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal dreamina request failed: path=%s: %w", path, err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("%w: new request failed", ErrInvalidRequest)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.shouldAddDreaminaBusinessHeaders(path) {
		req.Header.Set(HeaderAppID, c.aid)
		req.Header.Set(HeaderPF, DefaultPF)
		req.Header.Set(HeaderTTLogID, newTTLogID())
	}
	return req, nil
}

func (c *Client) authorize(req *http.Request) error {
	credential, err := c.provider.GetCredential(req.Context())
	if err != nil {
		return fmt.Errorf("get dreamina credential failed: %w", err)
	}
	if err := credential.Validate(); err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	proof, err := authsign.Sign(req.Method, req.URL.EscapedPath(), credential.AccessToken, credential.DeviceKey, c.signOptions)
	if err != nil {
		return fmt.Errorf("sign dreamina request failed: method=%s path=%s: %w", req.Method, req.URL.EscapedPath(), err)
	}
	proof.ApplyTo(req.Header)
	return nil
}

func (c *Client) defaultQuery() url.Values {
	values := url.Values{}
	if c.agentDetect != "" {
		values.Set("agent_detect", c.agentDetect)
	}
	if c.aid != "" {
		values.Set("aid", c.aid)
	}
	if c.cliVersion != "" {
		values.Set("cli_version", c.cliVersion)
	}
	if c.from != "" {
		values.Set("from", c.from)
	}
	return values
}

func (c *Client) shouldAddDreaminaBusinessHeaders(path string) bool {
	switch path {
	case EndpointImageGenerate, EndpointHistoryByIDs, EndpointUploadToken, EndpointResourceStore:
		return true
	default:
		return false
	}
}

func newTTLogID() string {
	return common.GetUUID() + "0"
}
