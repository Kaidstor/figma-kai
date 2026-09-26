package command

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Kaidstor/figma-kai/internal/exit"
	"github.com/Kaidstor/figma-kai/internal/figma"
	"github.com/Kaidstor/figma-kai/internal/output"
)

type nodeData struct {
	FileKey string        `json:"file_key"`
	URL     string        `json:"url"`
	Chain   string        `json:"chain,omitempty"`
	Tree    figma.Compact `json:"tree"`
}

func cmdNode(ctx context.Context, p *output.Printer, args []string) int {
	fs := newFlagSet("node")
	depth := fs.Int("depth", 3, "")
	withPath := fs.Bool("path", false, "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return p.Fail("node", exit.Tool, "usage", "%s", err)
	}
	if len(pos) != 1 {
		return p.Fail("node", exit.Tool, "usage", "нужна одна ссылка: figma-kai node <ссылка>")
	}
	if *depth < 1 || *depth > 10 {
		return p.Fail("node", exit.Tool, "usage", "--depth должен быть в пределах 1–10")
	}

	cli, _, ref, code, ok := setup(p, "node", pos[0])
	if !ok {
		return code
	}

	// Глубина запроса на единицу больше выдачи: иначе у последнего уровня не
	// видно, есть ли у него дети, и more_depth не проставить.
	root, err := cli.Node(ctx, ref.FileKey, ref.NodeID, *depth+1)
	if err != nil {
		return fail(p, "node", err)
	}

	data := nodeData{
		FileKey: ref.FileKey,
		URL:     ref.URL(),
		Tree:    figma.Compress(root, root.AbsoluteBoundingBox, 0, *depth),
	}
	if *withPath {
		chain, err := cli.Ancestors(ctx, ref.FileKey, ref.NodeID)
		if err != nil {
			return fail(p, "node", err)
		}
		data.Chain = chainText(chain, root)
	}
	if hasMoreDepth(data.Tree) {
		p.Warn("есть слои глубже --depth %d: смотри more_depth, увеличь глубину или спроси id поддерева", *depth)
	}

	return p.Result("node", exit.OK, data, func(w io.Writer) {
		if data.Chain != "" {
			fmt.Fprintln(w, data.Chain)
		}
		printTree(w, data.Tree, 0)
	})
}

func hasMoreDepth(c figma.Compact) bool {
	if c.MoreDepth {
		return true
	}
	for _, ch := range c.Children {
		if hasMoreDepth(ch) {
			return true
		}
	}
	return false
}

func printTree(w io.Writer, c figma.Compact, level int) {
	parts := []string{c.Type, "«" + c.Name + "»", c.ID}
	if c.Size != "" {
		parts = append(parts, c.Size)
	}
	if c.X != nil && c.Y != nil {
		parts = append(parts, fmt.Sprintf("@%d,%d", *c.X, *c.Y))
	}
	if c.Hidden {
		parts = append(parts, "скрыт")
	}
	if len(c.Fill) > 0 {
		parts = append(parts, "fill "+strings.Join(c.Fill, ", "))
	}
	if len(c.Stroke) > 0 {
		parts = append(parts, "stroke "+strings.Join(c.Stroke, " "))
	}
	if c.Radius > 0 {
		parts = append(parts, fmt.Sprintf("radius %g", c.Radius))
	}
	if c.Opacity != nil {
		parts = append(parts, fmt.Sprintf("opacity %g", *c.Opacity))
	}
	if c.Layout != "" {
		parts = append(parts, c.Layout)
	}
	if c.Font != "" {
		parts = append(parts, c.Font)
	}
	if c.Text != "" {
		parts = append(parts, fmt.Sprintf("%q", c.Text))
	}
	if c.MoreDepth {
		parts = append(parts, "…")
	}
	fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", level), strings.Join(parts, " "))
	for _, ch := range c.Children {
		printTree(w, ch, level+1)
	}
}
