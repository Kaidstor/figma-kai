package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Kaidstor/figma-kai/internal/config"
	"github.com/Kaidstor/figma-kai/internal/exit"
	"github.com/Kaidstor/figma-kai/internal/figma"
	"github.com/Kaidstor/figma-kai/internal/output"
)

type configData struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	FileKey  string `json:"file_key"`
	OutDir   string `json:"out_dir"`
	TokenRef string `json:"token_ref"`
}

func cmdConfig(p *output.Printer, args []string) int {
	path := config.Path()

	if len(args) > 0 && args[0] == "init" {
		if _, err := os.Stat(path); err == nil {
			return p.Fail("config", exit.Tool, "usage", "%s уже есть — правь его руками", path)
		}
		if err := config.Save(config.Template(), path); err != nil {
			return p.Fail("config", exit.Tool, "config", "%s", err)
		}
		return p.Result("config", exit.OK, map[string]string{"path": path}, func(w io.Writer) {
			fmt.Fprintf(w, "создан %s — заполни token_ref и, если нужно, file_key\n", path)
		})
	}
	if len(args) > 0 {
		return p.Fail("config", exit.Tool, "usage", "config [init]")
	}

	cfg, err := config.Load()
	if err != nil {
		return p.Fail("config", exit.Tool, "config", "%s", err)
	}
	data := configData{
		Path:     path,
		Exists:   cfg.Path != "",
		FileKey:  cfg.FileKey,
		OutDir:   cfg.ImageDir(),
		TokenRef: cfg.TokenRef,
	}
	return p.Result("config", exit.OK, data, func(w io.Writer) {
		fmt.Fprintf(w, "файл:      %s (есть: %t)\n", data.Path, data.Exists)
		fmt.Fprintf(w, "file_key:  %s\n", orDash(data.FileKey))
		fmt.Fprintf(w, "out_dir:   %s\n", data.OutDir)
		fmt.Fprintf(w, "token_ref: %s\n", orDash(data.TokenRef))
	})
}

// check — строка doctor в общей для семьи CLI форме.
type check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Details string `json:"details"`

	// optional — провал не меняет код выхода; в --human помечается warn
	optional bool
}

// cmdDoctor отвечает на вопрос «почему не работает»: где настройки, какой
// токен подставился и видит ли он файл по умолчанию. Обязательная проверка
// не прошла — код 2 (таймаут — 4); необязательная — ok: false и строка в
// warning, код не меняется.
func cmdDoctor(ctx context.Context, p *output.Printer, _ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return p.Fail("doctor", exit.Tool, "config", "%s", err)
	}
	token, err := cfg.LoadToken()
	if err != nil {
		return p.Fail("doctor", exit.Tool, "auth", "%s", err)
	}
	cli := figma.New(token.Value, config.RequestTimeout)

	configDetails := config.Path()
	if cfg.Path == "" {
		configDetails += " (файла нет, только окружение)"
	}
	checks := []check{
		{Name: "config", OK: true, Details: configDetails},
		{Name: "token", OK: true, Details: token.Source + ", " + config.Mask(token.Value)},
	}
	code := exit.OK

	// Гоча: токену с одними file_*:read /v1/me отвечает 403 Invalid scope,
	// хотя макеты он читает. Владелец — справка, настоящая проверка — файл
	me, err := cli.Me(ctx)
	switch {
	case err == nil:
		details := me.Handle
		if me.Email != "" {
			details += " <" + me.Email + ">"
		}
		checks = append(checks, check{Name: "user", OK: true, Details: details})
	case figma.KindOf(err) == figma.KindAuth:
		checks = append(checks, check{Name: "user", Details: "не узнать: нет scope current_user:read", optional: true})
		p.Warn("владельца токена не узнать (нет scope current_user:read) — для макетов это не нужно")
	default:
		return fail(p, "doctor", fmt.Errorf("токен из %s: %w", token.Source, err))
	}

	switch {
	case cfg.FileKey == "":
		checks = append(checks, check{Name: "file", Details: "file_key не задан — голые id узлов не сработают", optional: true})
		p.Warn("file_key не задан: голые id узлов не сработают, ссылки — да")
	default:
		meta, err := cli.File(ctx, cfg.FileKey)
		switch {
		case err == nil:
			checks = append(checks, check{Name: "file", OK: true,
				Details: fmt.Sprintf("%s «%s», правка %s", cfg.FileKey, meta.Name, meta.LastModified)})
		case figma.KindOf(err) == figma.KindTimeout:
			checks = append(checks, check{Name: "file", Details: err.Error()})
			code = exit.Timeout
		default:
			checks = append(checks, check{Name: "file", Details: err.Error()})
			code = exit.Tool
		}
	}

	return p.Result("doctor", code, map[string]any{"checks": checks}, func(w io.Writer) {
		for _, ch := range checks {
			mark := "fail"
			switch {
			case ch.OK:
				mark = "ok  "
			case ch.optional:
				mark = "warn"
			}
			fmt.Fprintf(w, "%s  %-7s %s\n", mark, ch.Name, ch.Details)
		}
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
