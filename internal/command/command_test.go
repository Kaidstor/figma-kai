package command

import (
	"testing"

	"github.com/Kaidstor/figma-kai/internal/figma"
)

func TestNearestContainer(t *testing.T) {
	chain := []figma.Node{
		{ID: "0:0", Type: "DOCUMENT"},
		{ID: "1:0", Type: "CANVAS", Name: "Макеты"},
		{ID: "1:1", Type: "FRAME", Name: "Главная"},
		{ID: "1:5", Type: "GROUP", Name: "Строка"},
	}
	got, ok := nearestContainer(chain)
	if !ok || got.ID != "1:5" {
		t.Fatalf("ближайший контейнер: %+v %v", got, ok)
	}

	top, ok := topFrame(chain)
	if !ok || top.ID != "1:1" {
		t.Fatalf("фрейм верхнего уровня: %+v %v", top, ok)
	}

	if _, ok := nearestContainer(chain[:2]); ok {
		t.Fatal("узел прямо на странице: подниматься некуда")
	}
}

func TestChainText(t *testing.T) {
	chain := []figma.Node{
		{Type: "DOCUMENT", Name: "Document"},
		{Type: "CANVAS", Name: "Макеты"},
		{Type: "FRAME", Name: "Главная"},
	}
	got := chainText(chain, figma.Node{Name: "Rectangle 12"})
	if got != "Макеты › Главная › Rectangle 12" {
		t.Fatal(got)
	}
	if chainText(nil, figma.Node{Name: "x"}) != "" {
		t.Fatal("без цепочки — пусто")
	}
}

func TestSplitGlobalFlags(t *testing.T) {
	human, rest := splitGlobalFlags([]string{"shot", "--human", "link", "--self"})
	if !human || len(rest) != 3 || rest[2] != "--self" {
		t.Fatalf("%v %v", human, rest)
	}
}

func TestParseArgsFlagsAfterPositional(t *testing.T) {
	fs := newFlagSet("shot")
	self := fs.Bool("self", false, "")
	pos, err := parseArgs(fs, []string{"link", "--self"})
	if err != nil || !*self || len(pos) != 1 || pos[0] != "link" {
		t.Fatalf("%v %v %v", pos, *self, err)
	}
}
