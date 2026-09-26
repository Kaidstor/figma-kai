// Package command — CLI-слой: роутер, разбор флагов и сами команды.
package command

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kaidstor/figma-kai/internal/config"
	"github.com/Kaidstor/figma-kai/internal/exit"
	"github.com/Kaidstor/figma-kai/internal/figma"
	"github.com/Kaidstor/figma-kai/internal/output"
)

// version подставляется ldflags при сборке; в go run остаётся dev.
var version = "dev"

// commandTimeout ограничивает команду целиком: внутри клиент повторяет
// запросы на 429, и без общего срока залипшая Figma держала бы вызов вечно.
const commandTimeout = 3 * time.Minute

const usage = `figma-kai — макеты Figma из терминала и из агента

Использование:
  figma-kai <команда> [аргументы]

<ссылка> — ссылка из браузера или задачи (…figma.com/design/<key>/…?node-id=1-2),
пара <key>/<node> или голый id узла 1:2 (файл тогда берётся из file_key настроек).

Команды:
  shot <ссылка>                 PNG узла; печатает путь к файлу. Если ссылка ведёт на
                                подложку, текст или вектор, снимается ближайший фрейм
    --screen                    снять экран целиком: фрейм верхнего уровня страницы
    --self                      снять ровно указанный слой, без подъёма к фрейму
    --scale <N>                 масштаб 0.01–4 (по умолчанию 2)
    --out <файл>                куда сохранить (по умолчанию <out_dir>/<key>-<node>@<N>x.png)

  node <ссылка>                 дерево слоёв: тип, имя, id, размер, x/y от корня, заливка,
                                обводка, радиус, текст и шрифт, автолейаут (gap, pad)
    --depth <N>                 уровни детей (по умолчанию 3); глубже — more_depth
    --path                      добавить цепочку предков: страница › фрейм › слой

Служебное:
  config [init]                 показать настройки или создать шаблон
  doctor                        токен, владелец токена, доступ к file_key
  version

Настройки — ` + "`~/.config/figma-kai/config.json`" + ` (file_key, out_dir, token_ref), поверх него
переменные FIGMA_FILE_KEY, FIGMA_KAI_OUT_DIR, FIGMA_TOKEN_REF. Токен — $FIGMA_TOKEN
или sec по token_ref; в argv он не попадает.

Общие флаги:
  --human                       вывод для человека вместо JSON
  --json                        машиночитаемый вывод (по умолчанию)
  -h, --help                    справка

Коды выхода:
  0 сделано                     2 ошибка инструмента, аргументов или токена   4 Figma не ответила
  1 Figma не отрендерила узел   3 нет файла или узла (или доступа к нему)`

// Run разбирает argv и возвращает код выхода. Ошибки печатает сам — наверх
// уходит только число.
func Run(args []string) int {
	human, args := splitGlobalFlags(args)

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(usage)
		return exit.OK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	p := output.New(human)
	name, rest := args[0], args[1:]

	switch name {
	case "shot":
		return cmdShot(ctx, p, rest)
	case "node":
		return cmdNode(ctx, p, rest)
	case "config":
		return cmdConfig(p, rest)
	case "doctor":
		return cmdDoctor(ctx, p, rest)
	case "version", "--version":
		return p.Result("version", exit.OK,
			map[string]string{"version": version},
			func(w io.Writer) { fmt.Fprintln(w, version) })
	default:
		return p.Fail("", exit.Tool, "usage",
			"неизвестная команда %q; figma-kai --help покажет список", name)
	}
}

// splitGlobalFlags вынимает общие флаги из любого места argv: подкоманды
// разбирают остаток своим FlagSet, а он падает на неизвестном флаге.
func splitGlobalFlags(args []string) (human bool, rest []string) {
	for _, a := range args {
		switch a {
		case "--human":
			human = true
		case "--json":
			human = false
		default:
			rest = append(rest, a)
		}
	}
	return human, rest
}

// parseArgs разбирает флаги, встречающиеся и после позиционных аргументов.
//
// Гоча: стандартный flag.Parse останавливается на первом не-флаге, и
// `figma-kai shot <ссылка> --self` молча снимал бы фрейм вместо слоя.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// setup отдаёт клиент, настройки и разобранную ссылку — общий вход shot и node.
func setup(p *output.Printer, command, raw string) (*figma.Client, config.Config, figma.Ref, int, bool) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cfg, figma.Ref{}, p.Fail(command, exit.Tool, "config", "%s", err), false
	}
	ref, err := figma.ParseRef(raw, cfg.FileKey)
	if errors.Is(err, figma.ErrNoNode) {
		return nil, cfg, ref, p.Fail(command, exit.Tool, "usage",
			"в ссылке нет node-id: в Figma выдели фрейм и возьми «Copy link to selection»"), false
	}
	if err != nil {
		return nil, cfg, ref, p.Fail(command, exit.Tool, "usage", "%s", err), false
	}
	token, err := cfg.LoadToken()
	if err != nil {
		return nil, cfg, ref, p.Fail(command, exit.Tool, "auth", "%s", err), false
	}
	return figma.New(token.Value, config.RequestTimeout), cfg, ref, 0, true
}

// fail переводит ошибку Figma в код выхода: агент решает по коду, подробности
// читает из конверта.
func fail(p *output.Printer, command string, err error) int {
	kind := figma.KindOf(err)
	code := exit.Tool
	switch kind {
	case figma.KindNotFound:
		code = exit.NotFound
	case figma.KindTimeout:
		code = exit.Timeout
	case figma.KindRender:
		code = exit.NotApplied
	}
	msg := err.Error()
	if kind == figma.KindAuth {
		msg += " — проверь токен: figma-kai doctor"
	}
	return p.Fail(command, code, kind, "%s", msg)
}
