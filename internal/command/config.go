package command

import (
	"context"
	"errors"
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

type doctorData struct {
	Config      string `json:"config"`
	TokenSource string `json:"token_source"`
	TokenMask   string `json:"token_mask"`
	User        string `json:"user,omitempty"`
	FileKey     string `json:"file_key,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	FileEdited  string `json:"file_edited,omitempty"`
}

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

	data := doctorData{
		Config:      config.Path(),
		TokenSource: token.Source,
		TokenMask:   config.Mask(token.Value),
	}
	// Гоча: токену с одними file_*:read /v1/me отвечает 403 Invalid scope, хотя
	// макеты он читает. Владелец — справка, настоящая проверка — доступ к файлу
	me, err := cli.Me(ctx)
	var meErr *figma.Error
	switch {
	case errors.As(err, &meErr) && meErr.Kind == figma.KindAuth && cfg.FileKey != "":
		p.Warn("владельца токена не узнать (%s) — для макетов это не нужно", meErr.Message)
	case err != nil:
		return fail(p, "doctor", fmt.Errorf("токен из %s: %w", token.Source, err))
	default:
		data.User = me.Handle
		if me.Email != "" {
			data.User += " <" + me.Email + ">"
		}
	}

	if cfg.FileKey != "" {
		data.FileKey = cfg.FileKey
		meta, err := cli.File(ctx, cfg.FileKey)
		if err != nil {
			return fail(p, "doctor", fmt.Errorf("file_key %s, токен из %s: %w", cfg.FileKey, token.Source, err))
		}
		data.FileName = meta.Name
		data.FileEdited = meta.LastModified
	}

	return p.Result("doctor", exit.OK, data, func(w io.Writer) {
		fmt.Fprintf(w, "настройки: %s\n", data.Config)
		fmt.Fprintf(w, "токен:     %s %s\n", data.TokenSource, data.TokenMask)
		fmt.Fprintf(w, "владелец:  %s\n", orDash(data.User))
		if data.FileName != "" {
			fmt.Fprintf(w, "файл:      %s «%s», правка %s\n", data.FileKey, data.FileName, data.FileEdited)
		}
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
