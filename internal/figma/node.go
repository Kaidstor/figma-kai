package figma

import (
	"fmt"
	"math"
	"strings"
)

// Node — слой Figma в объёме, нужном для сверки: остальное API тоже присылает,
// но разбирать его незачем.
type Node struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	Visible             *bool    `json:"visible,omitempty"`
	Children            []Node   `json:"children,omitempty"`
	AbsoluteBoundingBox *Rect    `json:"absoluteBoundingBox,omitempty"`
	Fills               []Paint  `json:"fills,omitempty"`
	Strokes             []Paint  `json:"strokes,omitempty"`
	StrokeWeight        float64  `json:"strokeWeight,omitempty"`
	CornerRadius        float64  `json:"cornerRadius,omitempty"`
	Opacity             *float64 `json:"opacity,omitempty"`
	Characters          string   `json:"characters,omitempty"`
	Style               *Type    `json:"style,omitempty"`
	LayoutMode          string   `json:"layoutMode,omitempty"`
	ItemSpacing         float64  `json:"itemSpacing,omitempty"`
	PaddingTop          float64  `json:"paddingTop,omitempty"`
	PaddingRight        float64  `json:"paddingRight,omitempty"`
	PaddingBottom       float64  `json:"paddingBottom,omitempty"`
	PaddingLeft         float64  `json:"paddingLeft,omitempty"`
	ComponentID         string   `json:"componentId,omitempty"`
}

// Rect — положение и размер в координатах холста.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Paint — заливка или обводка; разбираются только сплошные цвета.
type Paint struct {
	Type    string   `json:"type"`
	Visible *bool    `json:"visible,omitempty"`
	Opacity *float64 `json:"opacity,omitempty"`
	Color   *Color   `json:"color,omitempty"`
}

// Color — компоненты 0..1, как их отдаёт API.
type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

// Type — стиль текста.
type Type struct {
	FontFamily   string  `json:"fontFamily"`
	FontWeight   float64 `json:"fontWeight"`
	FontSize     float64 `json:"fontSize"`
	LineHeightPx float64 `json:"lineHeightPx"`
}

// Hidden — слой выключен в макете.
func (n Node) Hidden() bool { return n.Visible != nil && !*n.Visible }

// Size — «1440×900» или пусто, если размеров нет (страница).
func (n Node) Size() string {
	if n.AbsoluteBoundingBox == nil {
		return ""
	}
	return fmt.Sprintf("%s×%s", num(n.AbsoluteBoundingBox.Width), num(n.AbsoluteBoundingBox.Height))
}

// Compact — слой в виде, который удобно читать агенту: цвета в hex, отступы
// числами, координаты относительно корня выдачи, а не холста.
type Compact struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Size      string    `json:"size,omitempty"`
	X         *int      `json:"x,omitempty"`
	Y         *int      `json:"y,omitempty"`
	Hidden    bool      `json:"hidden,omitempty"`
	Fill      []string  `json:"fill,omitempty"`
	Stroke    []string  `json:"stroke,omitempty"`
	Radius    float64   `json:"radius,omitempty"`
	Opacity   *float64  `json:"opacity,omitempty"`
	Text      string    `json:"text,omitempty"`
	Font      string    `json:"font,omitempty"`
	Layout    string    `json:"layout,omitempty"`
	Children  []Compact `json:"children,omitempty"`
	MoreDepth bool      `json:"more_depth,omitempty"`
}

// TextLimit — сколько символов текста слоя отдавать: длинные абзацы макета
// только раздувают вывод.
const TextLimit = 200

// Compress сворачивает дерево; origin — левый верхний угол корня.
// maxDepth — уровни детей после корня; глубже ставится more_depth.
func Compress(n Node, origin *Rect, depth, maxDepth int) Compact {
	c := Compact{
		ID:     n.ID,
		Type:   n.Type,
		Name:   n.Name,
		Size:   n.Size(),
		Hidden: n.Hidden(),
		Fill:   colors(n.Fills),
		Stroke: colors(n.Strokes),
		Radius: n.CornerRadius,
		Layout: layout(n),
	}
	if n.Opacity != nil && *n.Opacity < 1 {
		o := round2(*n.Opacity)
		c.Opacity = &o
	}
	if len(c.Stroke) > 0 && n.StrokeWeight > 0 {
		c.Stroke = append(c.Stroke, num(n.StrokeWeight)+"px")
	}
	if origin != nil && n.AbsoluteBoundingBox != nil && depth > 0 {
		x := int(math.Round(n.AbsoluteBoundingBox.X - origin.X))
		y := int(math.Round(n.AbsoluteBoundingBox.Y - origin.Y))
		c.X, c.Y = &x, &y
	}
	if n.Type == "TEXT" {
		c.Text = truncate(n.Characters, TextLimit)
		c.Font = font(n.Style)
	}

	if depth >= maxDepth {
		c.MoreDepth = len(n.Children) > 0
		return c
	}
	for _, ch := range n.Children {
		c.Children = append(c.Children, Compress(ch, origin, depth+1, maxDepth))
	}
	return c
}

func colors(paints []Paint) []string {
	var out []string
	for _, p := range paints {
		if p.Visible != nil && !*p.Visible {
			continue
		}
		if p.Type != "SOLID" || p.Color == nil {
			out = append(out, strings.ToLower(p.Type))
			continue
		}
		hex := fmt.Sprintf("#%02X%02X%02X", byte255(p.Color.R), byte255(p.Color.G), byte255(p.Color.B))
		alpha := p.Color.A
		if p.Opacity != nil {
			alpha *= *p.Opacity
		}
		if alpha < 0.995 {
			hex += fmt.Sprintf(" %d%%", int(math.Round(alpha*100)))
		}
		out = append(out, hex)
	}
	return out
}

func layout(n Node) string {
	if n.LayoutMode == "" || n.LayoutMode == "NONE" {
		return ""
	}
	dir := "row"
	if n.LayoutMode == "VERTICAL" {
		dir = "column"
	}
	s := dir
	if n.ItemSpacing > 0 {
		s += " gap " + num(n.ItemSpacing)
	}
	if n.PaddingTop+n.PaddingRight+n.PaddingBottom+n.PaddingLeft > 0 {
		s += fmt.Sprintf(" pad %s/%s/%s/%s",
			num(n.PaddingTop), num(n.PaddingRight), num(n.PaddingBottom), num(n.PaddingLeft))
	}
	return s
}

func font(t *Type) string {
	if t == nil {
		return ""
	}
	s := fmt.Sprintf("%s %s", t.FontFamily, num(t.FontSize))
	if t.LineHeightPx > 0 {
		s += "/" + num(t.LineHeightPx)
	}
	if t.FontWeight > 0 {
		s += " " + num(t.FontWeight)
	}
	return s
}

func byte255(v float64) int { return int(math.Round(v * 255)) }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// num печатает число без хвоста нулей: 14, 13.5.
func num(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%d", int(v))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}
