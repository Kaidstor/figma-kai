package command

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Kaidstor/figma-kai/internal/exit"
	"github.com/Kaidstor/figma-kai/internal/figma"
	"github.com/Kaidstor/figma-kai/internal/output"
)

// leafTypes — слои, которые сами по себе экраном не бывают. Ссылку на такой
// слой дают по ошибке: кликнули по фону фрейма и скопировали ссылку — тогда
// вместо экрана приходит белая подложка.
var leafTypes = map[string]bool{
	"RECTANGLE": true, "ELLIPSE": true, "VECTOR": true, "LINE": true, "STAR": true,
	"REGULAR_POLYGON": true, "BOOLEAN_OPERATION": true, "TEXT": true,
}

// containerTypes — куда подниматься с листового слоя.
var containerTypes = map[string]bool{
	"FRAME": true, "COMPONENT": true, "COMPONENT_SET": true, "INSTANCE": true,
	"SECTION": true, "GROUP": true,
}

type nodeInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size string `json:"size,omitempty"`
}

func infoOf(n figma.Node) nodeInfo {
	return nodeInfo{ID: n.ID, Name: n.Name, Type: n.Type, Size: n.Size()}
}

type shotData struct {
	Path      string    `json:"path"`
	FileKey   string    `json:"file_key"`
	Node      nodeInfo  `json:"node"`
	Requested *nodeInfo `json:"requested,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Chain     string    `json:"chain,omitempty"`
	Scale     float64   `json:"scale"`
	URL       string    `json:"url"`
}

func cmdShot(ctx context.Context, p *output.Printer, args []string) int {
	fs := newFlagSet("shot")
	self := fs.Bool("self", false, "")
	screen := fs.Bool("screen", false, "")
	scale := fs.Float64("scale", 2, "")
	out := fs.String("out", "", "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return p.Fail("shot", exit.Tool, "usage", "%s", err)
	}
	if len(pos) != 1 {
		return p.Fail("shot", exit.Tool, "usage", "нужна одна ссылка: figma-kai shot <ссылка>")
	}
	if *self && *screen {
		return p.Fail("shot", exit.Tool, "usage", "--self и --screen взаимоисключающие")
	}
	if *scale < 0.01 || *scale > 4 {
		return p.Fail("shot", exit.Tool, "usage", "--scale должен быть в пределах 0.01–4")
	}

	cli, cfg, ref, code, ok := setup(p, "shot", pos[0])
	if !ok {
		return code
	}

	requested, err := cli.Node(ctx, ref.FileKey, ref.NodeID, 1)
	if err != nil {
		return fail(p, "shot", err)
	}
	if requested.Type == "CANVAS" || requested.Type == "DOCUMENT" {
		return p.Fail("shot", exit.Tool, "usage",
			"%s «%s» — это страница целиком, Figma её не рендерит; фреймы страницы: figma-kai node %s --depth 1",
			requested.Type, requested.Name, pos[0])
	}

	target := requested
	var chain []figma.Node
	var reason string

	if *screen || (!*self && leafTypes[requested.Type]) {
		chain, err = cli.Ancestors(ctx, ref.FileKey, ref.NodeID)
		if err != nil {
			return fail(p, "shot", err)
		}
		switch {
		case *screen:
			if top, found := topFrame(chain); found {
				target = top
				reason = "--screen: снят фрейм верхнего уровня"
			}
		default:
			if parent, found := nearestContainer(chain); found {
				target = parent
				reason = fmt.Sprintf("ссылка ведёт на %s «%s» — снят ближайший фрейм; ровно этот слой: --self",
					requested.Type, requested.Name)
			}
		}
	}

	link, err := cli.ImageURL(ctx, ref.FileKey, target.ID, "png", *scale)
	if err != nil {
		return fail(p, "shot", err)
	}

	path := *out
	if path == "" {
		name := fmt.Sprintf("%s-%s@%sx.png", ref.FileKey,
			strings.NewReplacer(":", "-", ";", "_").Replace(target.ID),
			strconv.FormatFloat(*scale, 'f', -1, 64))
		path = filepath.Join(cfg.ImageDir(), name)
	}
	if err := cli.Download(ctx, link, path); err != nil {
		return fail(p, "shot", err)
	}

	data := shotData{
		Path:    path,
		FileKey: ref.FileKey,
		Node:    infoOf(target),
		Reason:  reason,
		Chain:   chainText(chain, requested),
		Scale:   *scale,
		URL:     figma.Ref{FileKey: ref.FileKey, NodeID: target.ID}.URL(),
	}
	if target.ID != requested.ID {
		r := infoOf(requested)
		data.Requested = &r
	}

	return p.Result("shot", exit.OK, data, func(w io.Writer) {
		fmt.Fprintln(w, path)
		fmt.Fprintf(w, "%s «%s» %s %s\n", target.Type, target.Name, target.ID, target.Size())
		if data.Chain != "" {
			fmt.Fprintln(w, data.Chain)
		}
		if reason != "" {
			fmt.Fprintln(w, reason)
		}
	})
}

// nearestContainer — ближайший предок-контейнер ниже страницы.
func nearestContainer(chain []figma.Node) (figma.Node, bool) {
	for i := len(chain) - 1; i >= 0; i-- {
		switch {
		case chain[i].Type == "CANVAS" || chain[i].Type == "DOCUMENT":
			return figma.Node{}, false
		case containerTypes[chain[i].Type]:
			return chain[i], true
		}
	}
	return figma.Node{}, false
}

// topFrame — предок, лежащий прямо на странице. Если сам узел лежит на
// странице, предка нет: снимается он сам.
func topFrame(chain []figma.Node) (figma.Node, bool) {
	for i := 0; i+1 < len(chain); i++ {
		if chain[i].Type == "CANVAS" {
			return chain[i+1], true
		}
	}
	return figma.Node{}, false
}

// chainText — «Макеты › Главная › Rectangle 12», без документа.
func chainText(chain []figma.Node, leaf figma.Node) string {
	if len(chain) == 0 {
		return ""
	}
	var names []string
	for _, n := range chain {
		if n.Type == "DOCUMENT" {
			continue
		}
		names = append(names, n.Name)
	}
	names = append(names, leaf.Name)
	return strings.Join(names, " › ")
}
