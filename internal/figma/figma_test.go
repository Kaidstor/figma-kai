package figma

import (
	"errors"
	"testing"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		in, def, key, node string
	}{
		{"https://www.figma.com/design/AbC123/FILE?node-id=1-2&t=x-11", "", "AbC123", "1:2"},
		{"https://www.figma.com/file/AbC123/FILE?node-id=1%3A2", "", "AbC123", "1:2"},
		{"www.figma.com/design/AbC123/FILE?node-id=1-2", "", "AbC123", "1:2"},
		{"AbC123/1-2", "", "AbC123", "1:2"},
		{"3:4", "Def456", "Def456", "3:4"},
		{"I5-6;7-8", "Def456", "Def456", "I5:6;7:8"},
	}
	for _, c := range cases {
		got, err := ParseRef(c.in, c.def)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got.FileKey != c.key || got.NodeID != c.node {
			t.Fatalf("%s: got %+v", c.in, got)
		}
	}
}

func TestParseRefErrors(t *testing.T) {
	if _, err := ParseRef("https://www.figma.com/design/AbC123/FILE", ""); !errors.Is(err, ErrNoNode) {
		t.Fatalf("ссылка без node-id: %v", err)
	}
	if _, err := ParseRef("3:4", ""); err == nil {
		t.Fatal("голый id без file_key должен давать ошибку")
	}
	if _, err := ParseRef("https://example.com/x", ""); err == nil {
		t.Fatal("чужая ссылка должна давать ошибку")
	}
}

func TestRefURL(t *testing.T) {
	got := Ref{FileKey: "AbC123", NodeID: "3:4"}.URL()
	if got != "https://www.figma.com/design/AbC123/?node-id=3-4" {
		t.Fatal(got)
	}
}

func TestCompress(t *testing.T) {
	half := 0.5
	root := Node{
		ID: "1:1", Type: "FRAME", Name: "Экран",
		AbsoluteBoundingBox: &Rect{X: 100, Y: 200, Width: 1440, Height: 900},
		Fills:               []Paint{{Type: "SOLID", Color: &Color{R: 0.89, G: 0.94, B: 0.99, A: 1}}},
		LayoutMode:          "VERTICAL", ItemSpacing: 10, PaddingTop: 20, PaddingLeft: 20,
		Children: []Node{{
			ID: "1:2", Type: "TEXT", Name: "Заголовок", Characters: "Выгрузка отчетов",
			AbsoluteBoundingBox: &Rect{X: 120, Y: 230, Width: 160, Height: 24},
			Fills:               []Paint{{Type: "SOLID", Color: &Color{A: 1}, Opacity: &half}},
			Style:               &Type{FontFamily: "Inter", FontSize: 20, LineHeightPx: 24, FontWeight: 600},
			Children:            nil,
		}, {
			ID: "1:3", Type: "GROUP", Name: "Кнопки",
			AbsoluteBoundingBox: &Rect{X: 1000, Y: 230, Width: 80, Height: 32},
			Children:            []Node{{ID: "1:4", Type: "VECTOR", Name: "x"}},
		}},
	}

	c := Compress(root, root.AbsoluteBoundingBox, 0, 1)
	if c.Size != "1440×900" || c.X != nil {
		t.Fatalf("корень: %+v", c)
	}
	if len(c.Fill) != 1 || c.Fill[0] != "#E3F0FC" {
		t.Fatalf("заливка: %v", c.Fill)
	}
	if c.Layout != "column gap 10 pad 20/0/0/20" {
		t.Fatalf("layout: %q", c.Layout)
	}
	text := c.Children[0]
	if *text.X != 20 || *text.Y != 30 || text.Font != "Inter 20/24 600" || text.Fill[0] != "#000000 50%" {
		t.Fatalf("текст: %+v", text)
	}
	group := c.Children[1]
	if !group.MoreDepth || len(group.Children) != 0 {
		t.Fatalf("глубже maxDepth должны быть отрезаны: %+v", group)
	}
}

func TestPathTo(t *testing.T) {
	doc := Node{ID: "0:0", Type: "DOCUMENT", Children: []Node{
		{ID: "1:0", Type: "CANVAS", Name: "Макеты", Children: []Node{
			{ID: "1:1", Type: "FRAME", Name: "Главная", Children: []Node{
				{ID: "1:2", Type: "RECTANGLE", Name: "Фон"},
			}},
		}},
	}}
	chain := pathTo(doc, "1:2")
	if len(chain) != 3 || chain[2].ID != "1:1" || chain[2].Children != nil {
		t.Fatalf("цепочка: %+v", chain)
	}
	if pathTo(doc, "9:9") != nil {
		t.Fatal("чужой id должен давать nil")
	}
}
