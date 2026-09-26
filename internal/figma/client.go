// Package figma — клиент REST API Figma (api.figma.com) в объёме, который
// нужен для сверки с макетом: картинка узла, дерево слоёв, цепочка предков.
package figma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.figma.com"

// Классы ошибок: по ним CLI выбирает код выхода, не разбирая текст.
const (
	KindAuth     = "auth"
	KindNotFound = "not_found"
	KindTimeout  = "timeout"
	KindNetwork  = "network"
	KindAPI      = "api"
	KindRender   = "render"
)

// Error — отказ Figma с классом.
type Error struct {
	Kind    string
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("Figma %d: %s", e.Status, e.Message)
	}
	return e.Message
}

// KindOf достаёт класс ошибки; незнакомые считаются ошибкой API.
func KindOf(err error) string {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Kind
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	return KindNetwork
}

// Client ходит в API с персональным токеном.
type Client struct {
	token string
	http  *http.Client
}

func New(token string, timeout time.Duration) *Client {
	return &Client{token: token, http: &http.Client{Timeout: timeout}}
}

// User — владелец токена.
type User struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Email  string `json:"email"`
}

// Me — кто владелец токена.
func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.get(ctx, "/v1/me", nil, &u)
	return u, err
}

// FileMeta — имя файла и время последней правки.
type FileMeta struct {
	Name         string `json:"name"`
	LastModified string `json:"lastModified"`
	Version      string `json:"version"`
}

// File — метаданные файла; depth=1 отдаёт только страницы, без слоёв.
func (c *Client) File(ctx context.Context, key string) (FileMeta, error) {
	var m FileMeta
	err := c.get(ctx, "/v1/files/"+url.PathEscape(key), url.Values{"depth": {"1"}}, &m)
	return m, err
}

// Node — узел по id с поддеревом глубиной depth (0 — только сам узел).
func (c *Client) Node(ctx context.Context, key, id string, depth int) (Node, error) {
	q := url.Values{"ids": {id}}
	if depth > 0 {
		q.Set("depth", strconv.Itoa(depth))
	}
	var res struct {
		Nodes map[string]*struct {
			Document Node `json:"document"`
		} `json:"nodes"`
	}
	if err := c.get(ctx, "/v1/files/"+url.PathEscape(key)+"/nodes", q, &res); err != nil {
		return Node{}, err
	}
	n := res.Nodes[id]
	if n == nil {
		return Node{}, &Error{Kind: KindNotFound, Message: fmt.Sprintf("узла %s в файле нет", id)}
	}
	return n.Document, nil
}

// Ancestors — цепочка от страницы до родителя узла, без самого узла.
//
// Отдельного эндпоинта «родитель» у Figma нет. Работает так: /v1/files/<key>
// с ids отдаёт документ, обрезанный до ветвей к запрошенным узлам, и по нему
// ищется путь. Поддерево самого узла тоже приходит целиком, поэтому звать это
// только когда цепочка действительно нужна.
func (c *Client) Ancestors(ctx context.Context, key, id string) ([]Node, error) {
	var res struct {
		Document Node `json:"document"`
	}
	if err := c.get(ctx, "/v1/files/"+url.PathEscape(key), url.Values{"ids": {id}}, &res); err != nil {
		return nil, err
	}
	chain := pathTo(res.Document, id)
	if chain == nil {
		return nil, &Error{Kind: KindNotFound, Message: fmt.Sprintf("узла %s в файле нет", id)}
	}
	return chain, nil
}

func pathTo(n Node, id string) []Node {
	for _, ch := range n.Children {
		if ch.ID == id {
			return []Node{strip(n)}
		}
		if rest := pathTo(ch, id); rest != nil {
			return append([]Node{strip(n)}, rest...)
		}
	}
	return nil
}

// strip убирает детей: в цепочке предков нужны только имя, тип и размеры.
func strip(n Node) Node {
	n.Children = nil
	return n
}

// ImageURL — ссылка на отрендеренную картинку узла. Ссылка ведёт в S3 Figma и
// живёт ограниченное время.
func (c *Client) ImageURL(ctx context.Context, key, id, format string, scale float64) (string, error) {
	q := url.Values{
		"ids":    {id},
		"format": {format},
		"scale":  {strconv.FormatFloat(scale, 'f', -1, 64)},
	}
	var res struct {
		Err    *string            `json:"err"`
		Images map[string]*string `json:"images"`
	}
	if err := c.get(ctx, "/v1/images/"+url.PathEscape(key), q, &res); err != nil {
		return "", err
	}
	if res.Err != nil && *res.Err != "" {
		return "", &Error{Kind: KindAPI, Message: *res.Err}
	}
	// Гоча: невидимый или пустой узел Figma не рендерит и отвечает 200 с null
	// вместо ссылки — это не сбой сети, повтор не поможет.
	link := res.Images[id]
	if link == nil || *link == "" {
		return "", &Error{Kind: KindRender, Message: fmt.Sprintf(
			"Figma не отрендерила узел %s (скрытый, пустой или слишком большой)", id)}
	}
	return *link, nil
}

// Download сохраняет картинку по ссылке из ImageURL. Токен сюда не уходит:
// ссылка на S3 подписана сама.
func (c *Client) Download(ctx context.Context, link, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &Error{Kind: KindAPI, Status: resp.StatusCode, Message: "картинка не скачалась"}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// get делает GET с токеном и разбирает JSON. На 429 и 5xx повторяет до трёх
// раз: у Figma жёсткий лимит частоты, и Retry-After она присылает сама.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Figma-Token", c.token)

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal(body, out); err != nil {
				return &Error{Kind: KindAPI, Message: "ответ не JSON: " + snippet(body)}
			}
			return nil
		}

		lastErr = statusError(resp.StatusCode, body)
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return lastErr
		}
		if err := sleep(ctx, retryAfter(resp.Header.Get("Retry-After"), attempt)); err != nil {
			return err
		}
	}
	return lastErr
}

func statusError(status int, body []byte) error {
	var res struct {
		Err     string `json:"err"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &res)
	msg := res.Err
	if msg == "" {
		msg = res.Message
	}
	if msg == "" {
		msg = snippet(body)
	}

	kind := KindAPI
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		kind = KindAuth
	case status == http.StatusNotFound:
		kind = KindNotFound
	}
	return &Error{Kind: kind, Status: status, Message: msg}
}

func retryAfter(header string, attempt int) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && s > 0 {
		if s > 30 {
			s = 30
		}
		return time.Duration(s) * time.Second
	}
	return time.Duration(attempt+1) * 2 * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
