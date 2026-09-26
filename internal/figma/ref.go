package figma

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Ref — файл и узел, на которые указывает ссылка.
type Ref struct {
	FileKey string
	NodeID  string
}

// URL — ссылка на узел для человека.
func (r Ref) URL() string {
	return "https://www.figma.com/design/" + r.FileKey + "/?node-id=" +
		url.QueryEscape(strings.ReplaceAll(r.NodeID, ":", "-"))
}

var (
	// Путь ссылки: /design/<key>/…, /file/<key>/…, /proto/<key>/…, /board/<key>/…
	pathKey = regexp.MustCompile(`^/(?:design|file|proto|board)/([A-Za-z0-9]+)`)
	// Голый id узла: 3:4, 3-4, I5:6;7:8
	bareNode = regexp.MustCompile(`^I?\d+[:-]\d+(?:;\d+[:-]\d+)*$`)
)

// ErrNoNode — ссылка на файл без node-id: рисовать нечего.
var ErrNoNode = errors.New("в ссылке нет node-id")

// ParseRef понимает ссылку из браузера или задачи, пару «<key>/<node>» и
// голый id узла (тогда файл берётся из настроек).
//
// Гоча: в ссылке id узла пишется через дефис (node-id=1-2), а API ждёт
// двоеточие. Дефис в id самого узла не бывает, поэтому замена безопасна.
func ParseRef(raw, defaultKey string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Ref{}, errors.New("пустая ссылка")
	}

	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "www.figma.com") || strings.HasPrefix(raw, "figma.com") {
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil {
			return Ref{}, fmt.Errorf("не разобрать ссылку: %w", err)
		}
		m := pathKey.FindStringSubmatch(u.Path)
		if m == nil {
			return Ref{}, fmt.Errorf("в ссылке %q нет ключа файла (/design/<key>/…)", raw)
		}
		node := u.Query().Get("node-id")
		if node == "" {
			return Ref{FileKey: m[1]}, ErrNoNode
		}
		return Ref{FileKey: m[1], NodeID: normalizeNode(node)}, nil
	}

	if key, node, ok := strings.Cut(raw, "/"); ok && bareNode.MatchString(node) {
		return Ref{FileKey: key, NodeID: normalizeNode(node)}, nil
	}

	if bareNode.MatchString(raw) {
		if defaultKey == "" {
			return Ref{}, errors.New(
				"голый id узла требует file_key в настройках (figma-kai config) — или передай ссылку целиком")
		}
		return Ref{FileKey: defaultKey, NodeID: normalizeNode(raw)}, nil
	}

	return Ref{}, fmt.Errorf("не понял %q: нужна ссылка на Figma, <key>/<node> или id узла", raw)
}

func normalizeNode(node string) string {
	return strings.ReplaceAll(node, "-", ":")
}
