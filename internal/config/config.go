// Package config — файл Figma по умолчанию, каталог для картинок и поиск
// токена.
//
// В коде нет ни ключей файлов, ни ссылок на секреты: всё это читается из
// `~/.config/figma-kai/config.json` и переопределяется переменными окружения.
// Так репозиторий можно держать публичным.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RequestTimeout щедрый намеренно: рендер большого фрейма в /v1/images у Figma
// занимает десятки секунд.
const RequestTimeout = 90 * time.Second

// Config — то, что различается между людьми и командами.
type Config struct {
	// FileKey — файл по умолчанию: с ним вместо ссылки хватает id узла
	// («3:4»). Пустой — ссылка обязательна.
	FileKey string `json:"file_key"`
	// OutDir — куда складывать картинки; пустой — <tmp>/figma-kai.
	OutDir string `json:"out_dir"`
	// TokenRef — ссылка на персональный токен в sec, вида "<проект>/<KEY>".
	TokenRef string `json:"token_ref"`

	// Path — откуда прочитан файл; пустой, если файла нет.
	Path string `json:"-"`
}

// Token — значение и то, откуда оно взялось: при 403 doctor показывает
// источник, чтобы было видно, какой именно токен подставился.
type Token struct {
	Value  string
	Source string
}

// Load читает настройки: файл, поверх него — переменные окружения. Отсутствие
// файла не ошибка: ссылка из задачи несёт ключ файла сама, а токен может
// прийти из окружения.
func Load() (Config, error) {
	path := Path()

	var cfg Config
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("%s: не разобрать JSON: %w", path, err)
		}
		cfg.Path = path
	case !errors.Is(err, os.ErrNotExist):
		return Config{}, err
	}

	applyEnv(&cfg)
	return cfg, nil
}

// Path — путь к файлу настроек: $FIGMA_KAI_CONFIG, иначе $XDG_CONFIG_HOME или
// ~/.config.
//
// Гоча: os.UserConfigDir на macOS отдаёт ~/Library/Application Support, и файл
// оказывается не там, где его ищут руками.
func Path() string {
	if v := strings.TrimSpace(os.Getenv("FIGMA_KAI_CONFIG")); v != "" {
		return v
	}
	dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "figma-kai", "config.json")
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "figma-kai", "config.json")
}

// Template — заготовка файла настроек.
func Template() Config {
	return Config{FileKey: "", OutDir: "", TokenRef: ""}
}

// Save пишет настройки в файл, создавая каталог.
func Save(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// ImageDir — каталог для картинок.
func (c Config) ImageDir() string {
	if c.OutDir != "" {
		return c.OutDir
	}
	return filepath.Join(os.TempDir(), "figma-kai")
}

// LoadToken ищет токен: переменная окружения, затем sec по ссылке из настроек.
// Значение никуда не печатается — наружу уходят только источник и маска.
func (c Config) LoadToken() (Token, error) {
	if v := strings.TrimSpace(os.Getenv("FIGMA_TOKEN")); v != "" {
		return Token{Value: v, Source: "env:FIGMA_TOKEN"}, nil
	}

	if c.TokenRef != "" {
		v, err := fromSec(c.TokenRef)
		if err != nil {
			return Token{}, fmt.Errorf("sec %s: %w", c.TokenRef, err)
		}
		if v != "" {
			return Token{Value: v, Source: "sec:" + c.TokenRef}, nil
		}
	}

	return Token{}, fmt.Errorf(
		"токен не найден: задай $FIGMA_TOKEN или token_ref в %s", Path())
}

// Mask отдаёт представление, безопасное для чата и логов.
func Mask(value string) string {
	r := []rune(value)
	if len(r) < 8 {
		return fmt.Sprintf("(%d символов)", len(r))
	}
	return fmt.Sprintf("%s…%s (%d символов)", string(r[:2]), string(r[len(r)-2:]), len(r))
}

func applyEnv(cfg *Config) {
	for _, pair := range []struct {
		env    string
		target *string
	}{
		{"FIGMA_FILE_KEY", &cfg.FileKey},
		{"FIGMA_KAI_OUT_DIR", &cfg.OutDir},
		{"FIGMA_TOKEN_REF", &cfg.TokenRef},
	} {
		if v := strings.TrimSpace(os.Getenv(pair.env)); v != "" {
			*pair.target = v
		}
	}
}

func fromSec(ref string) (string, error) {
	bin, err := exec.LookPath("sec")
	if err != nil {
		return "", fmt.Errorf("sec не найден в PATH: %w", err)
	}
	out, err := exec.Command(bin, "get", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
