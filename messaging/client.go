package messaging

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// Client calls the messaging service through the openhost router, on behalf
// of the app whose token it holds.
type Client struct {
	RouterURL string
	AppToken  string
	// The [[services.v2.consumes]] shortname, "messaging" by default.
	Shortname string
	HTTP      *http.Client
}

// NewClient reads OPENHOST_ROUTER_URL and OPENHOST_APP_TOKEN, which openhost
// sets in every app container.
func NewClient() *Client {
	return &Client{
		RouterURL: strings.TrimRight(os.Getenv("OPENHOST_ROUTER_URL"), "/"),
		AppToken:  os.Getenv("OPENHOST_APP_TOKEN"),
		Shortname: "messaging",
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// ProviderInfo is one entry of the router's provider listing.
type ProviderInfo struct {
	AppID          string `json:"app_id"`
	AppName        string `json:"app_name"`
	Status         string `json:"status"`
	IsDefault      bool   `json:"is_default"`
	ServiceVersion string `json:"service_version"`
}

// Providers lists every app providing the messaging service.
func (c *Client) Providers(ctx context.Context) ([]ProviderInfo, error) {
	var out struct {
		Providers []ProviderInfo `json:"providers"`
	}
	u := c.RouterURL + "/api/services/v2/providers?service=" + url.QueryEscape(ServiceURL)
	err := c.do(ctx, http.MethodGet, u, "", nil, "", &out)
	return out.Providers, err
}

// Provider returns a client bound to one provider app.
func (c *Client) Provider(appID string) *Provider {
	return &Provider{c: c, AppID: appID}
}

// Provider is a client for one messaging app (one account).
type Provider struct {
	c     *Client
	AppID string
}

func (p *Provider) url(path string, q url.Values) string {
	u := p.c.RouterURL + "/api/services/v2/call/" + p.c.Shortname + "/" + strings.TrimLeft(path, "/")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (p *Provider) get(ctx context.Context, path string, q url.Values, out any) error {
	return p.c.do(ctx, http.MethodGet, p.url(path, q), p.AppID, nil, "", out)
}

func (p *Provider) sendJSON(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	return p.c.do(ctx, method, p.url(path, nil), p.AppID, r, "application/json", out)
}

func (p *Provider) Account(ctx context.Context) (*Account, error) {
	var a Account
	return &a, p.get(ctx, "v1/account", nil, &a)
}

type ChatsQuery struct {
	Limit    int
	Cursor   string
	Archived string // "include" | "exclude" | "only"; "" = exclude
}

func (p *Provider) Chats(ctx context.Context, q ChatsQuery) (*ChatPage, error) {
	v := url.Values{}
	setInt(v, "limit", q.Limit)
	setStr(v, "cursor", q.Cursor)
	setStr(v, "archived", q.Archived)
	var page ChatPage
	return &page, p.get(ctx, "v1/chats", v, &page)
}

func (p *Provider) Chat(ctx context.Context, id string) (*ChatDetail, error) {
	var c ChatDetail
	return &c, p.get(ctx, "v1/chats/"+url.PathEscape(id), nil, &c)
}

// MessagesQuery: at most one of Before, After, Around. None = newest page.
type MessagesQuery struct {
	Before, After string // cursors
	Around        string // message id
	Limit         int
}

func (p *Provider) Messages(ctx context.Context, chatID string, q MessagesQuery) (*MessagePage, error) {
	v := url.Values{}
	setStr(v, "before", q.Before)
	setStr(v, "after", q.After)
	setStr(v, "around", q.Around)
	setInt(v, "limit", q.Limit)
	var page MessagePage
	return &page, p.get(ctx, "v1/chats/"+url.PathEscape(chatID)+"/messages", v, &page)
}

func (p *Provider) Message(ctx context.Context, id string) (*Message, error) {
	var m Message
	return &m, p.get(ctx, "v1/messages/"+url.PathEscape(id), nil, &m)
}

type RecentQuery struct {
	Since, Until time.Time
	Limit        int
	Cursor       string
}

// Recent lists messages across all chats, newest first.
func (p *Provider) Recent(ctx context.Context, q RecentQuery) (*RecentMessages, error) {
	v := url.Values{}
	setTime(v, "since", q.Since)
	setTime(v, "until", q.Until)
	setInt(v, "limit", q.Limit)
	setStr(v, "cursor", q.Cursor)
	var out RecentMessages
	return &out, p.get(ctx, "v1/messages", v, &out)
}

func (p *Provider) Search(ctx context.Context, query, chatID string, limit int) (*SearchResults, error) {
	v := url.Values{"q": {query}}
	setStr(v, "chat_id", chatID)
	setInt(v, "limit", limit)
	var out SearchResults
	return &out, p.get(ctx, "v1/search", v, &out)
}

func (p *Provider) People(ctx context.Context, query string) (*People, error) {
	v := url.Values{}
	setStr(v, "q", query)
	var out People
	return &out, p.get(ctx, "v1/people", v, &out)
}

// File fetches attachment or avatar bytes by the url field the provider gave.
// The caller closes the body.
func (p *Provider) File(ctx context.Context, relURL string) (*http.Response, error) {
	req, err := p.c.request(ctx, http.MethodGet, p.url(relURL, nil), p.AppID, nil, "")
	if err != nil {
		return nil, err
	}
	resp, err := p.c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, decodeError(resp)
	}
	return resp, nil
}

// StartChat finds or creates a DM. Set exactly one field.
type StartChat struct {
	Address  string `json:"address,omitempty"`
	Phone    string `json:"phone,omitempty"`
	PersonID string `json:"person_id,omitempty"`
}

func (p *Provider) StartChat(ctx context.Context, req StartChat) (*Chat, error) {
	var c Chat
	return &c, p.sendJSON(ctx, http.MethodPost, "v1/chats", req, &c)
}

type File struct {
	Name        string
	ContentType string
	Data        io.Reader
}

type Send struct {
	Text     string
	ReplyTo  string // message id
	Files    []File
	ClientID string // idempotency key; set one and reuse it on retries
}

func (p *Provider) Send(ctx context.Context, chatID string, s Send) (*SentMessages, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"text": s.Text, "reply_to": s.ReplyTo, "client_id": s.ClientID} {
		if v != "" {
			mw.WriteField(k, v)
		}
	}
	for _, f := range s.Files {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="files[]"; filename=%q`, f.Name)}
		h["Content-Type"] = []string{cmp.Or(f.ContentType, "application/octet-stream")}
		part, err := mw.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, f.Data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	var out SentMessages
	err := p.c.do(ctx, http.MethodPost, p.url("v1/chats/"+url.PathEscape(chatID)+"/messages", nil),
		p.AppID, &buf, mw.FormDataContentType(), &out)
	return &out, err
}

func (p *Provider) Edit(ctx context.Context, messageID, text string) error {
	return p.sendJSON(ctx, http.MethodPatch, "v1/messages/"+url.PathEscape(messageID), map[string]string{"text": text}, nil)
}

// Unsend deletes a message for everyone (or discards a failed one).
func (p *Provider) Unsend(ctx context.Context, messageID string) error {
	return p.sendJSON(ctx, http.MethodDelete, "v1/messages/"+url.PathEscape(messageID), nil, nil)
}

func (p *Provider) Retry(ctx context.Context, messageID string) (*Message, error) {
	var m Message
	return &m, p.sendJSON(ctx, http.MethodPost, "v1/messages/"+url.PathEscape(messageID)+"/retry", nil, &m)
}

func (p *Provider) React(ctx context.Context, messageID, emoji string) error {
	return p.sendJSON(ctx, http.MethodPost, "v1/messages/"+url.PathEscape(messageID)+"/reactions", map[string]string{"emoji": emoji}, nil)
}

func (p *Provider) Unreact(ctx context.Context, messageID, emoji string) error {
	return p.sendJSON(ctx, http.MethodDelete, "v1/messages/"+url.PathEscape(messageID)+"/reactions/"+url.PathEscape(emoji), nil, nil)
}

// MarkRead marks the chat read up to upTo, or to the latest message if "".
func (p *Provider) MarkRead(ctx context.Context, chatID, upTo string) error {
	body := map[string]string{}
	if upTo != "" {
		body["up_to"] = upTo
	}
	return p.sendJSON(ctx, http.MethodPost, "v1/chats/"+url.PathEscape(chatID)+"/read", body, nil)
}

// LoadOlder asks the provider to fetch a page of older messages from the network (capability load_older). A chat
// event follows when they're stored.
func (p *Provider) LoadOlder(ctx context.Context, chatID string) error {
	return p.sendJSON(ctx, http.MethodPost, "v1/chats/"+url.PathEscape(chatID)+"/history", nil, nil)
}

func (p *Provider) Typing(ctx context.Context, chatID string, typing bool) error {
	return p.sendJSON(ctx, http.MethodPost, "v1/chats/"+url.PathEscape(chatID)+"/typing", map[string]bool{"typing": typing}, nil)
}

// Fan-out.

// ProviderChat is a chat tagged with the provider it lives in. (AppID, Chat.ID)
// is globally unique.
type ProviderChat struct {
	AppID string
	Chat
}

// ProviderError records a provider that failed during a fan-out.
type ProviderError struct {
	AppID string
	Err   error
}

// RunningProviders returns the providers whose app is running.
func (c *Client) RunningProviders(ctx context.Context) ([]*Provider, error) {
	infos, err := c.Providers(ctx)
	if err != nil {
		return nil, err
	}
	var out []*Provider
	for _, info := range infos {
		if info.Status == "running" {
			out = append(out, c.Provider(info.AppID))
		}
	}
	return out, nil
}

// AllChats fetches each provider's first page of chats and merges them by
// last_message_at, newest first. Providers that fail are reported, not fatal.
func (c *Client) AllChats(ctx context.Context, perProvider int) ([]ProviderChat, []ProviderError, error) {
	providers, err := c.RunningProviders(ctx)
	if err != nil {
		return nil, nil, err
	}
	var (
		mu     sync.Mutex
		chats  []ProviderChat
		failed []ProviderError
		wg     sync.WaitGroup
	)
	for _, p := range providers {
		wg.Go(func() {
			page, err := p.Chats(ctx, ChatsQuery{Limit: perProvider})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = append(failed, ProviderError{p.AppID, err})
				return
			}
			for _, ch := range page.Chats {
				chats = append(chats, ProviderChat{p.AppID, ch})
			}
		})
	}
	wg.Wait()
	slices.SortStableFunc(chats, func(a, b ProviderChat) int {
		return cmpTime(b.LastMessageAt, a.LastMessageAt)
	})
	return chats, failed, nil
}

func cmpTime(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return a.Compare(*b)
}

// HTTP plumbing.

func (c *Client) request(ctx context.Context, method, u, appID string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AppToken)
	if appID != "" {
		req.Header.Set("X-OpenHost-Provider", appID)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func (c *Client) do(ctx context.Context, method, u, appID string, body io.Reader, contentType string, out any) error {
	req, err := c.request(ctx, method, u, appID, body, contentType)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return decodeError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeError(resp *http.Response) error {
	var e Error
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if json.Unmarshal(b, &e) != nil || e.Code == "" {
		return &Error{Code: fmt.Sprintf("http_%d", resp.StatusCode), Detail: strings.TrimSpace(string(b))}
	}
	return &e
}

func setStr(v url.Values, k, s string) {
	if s != "" {
		v.Set(k, s)
	}
}

func setInt(v url.Values, k string, n int) {
	if n > 0 {
		v.Set(k, fmt.Sprint(n))
	}
}

func setTime(v url.Values, k string, t time.Time) {
	if !t.IsZero() {
		v.Set(k, t.UTC().Format("2006-01-02T15:04:05.000Z07:00"))
	}
}
