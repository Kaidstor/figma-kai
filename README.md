# figma-kai

Макеты Figma из терминала и из агента: картинка экрана по ссылке из задачи и компактное дерево слоёв с размерами, цветами, текстом и отступами. REST API Figma, персональный токен, без MCP.

```sh
figma-kai shot 'https://www.figma.com/design/AbC123/File?node-id=1-2' --human
# /tmp/figma-kai/AbC123-3-4@2x.png
# FRAME «Экран» 3:4 1440×900
# Страница › Экран › Rectangle 12
# ссылка ведёт на RECTANGLE «Rectangle 12» — снят ближайший фрейм; ровно этот слой: --self

figma-kai node 3:4 --depth 2 --human
```

## Установка

```sh
brew install kaidstor/tap/figma-kai
```

Из исходников (нужен Go 1.26). Без `-ldflags` `figma-kai version` отвечает `dev`:

```sh
V=v0.1.0
go install -ldflags "-X github.com/Kaidstor/figma-kai/internal/command.version=$V" github.com/Kaidstor/figma-kai@$V
```

Первый запуск:

```sh
figma-kai config init      # ~/.config/figma-kai/config.json
figma-kai doctor --human   # токен, доступ к file_key
```

Настройки: `token_ref` — ссылка на токен в [sec](https://github.com/Kaidstor/sec) (`<проект>/<KEY>`), `file_key` — файл по умолчанию для голых id узлов, `out_dir` — каталог картинок. Вместо файла — переменные `FIGMA_TOKEN`, `FIGMA_TOKEN_REF`, `FIGMA_FILE_KEY`, `FIGMA_KAI_OUT_DIR`.

Токен: Figma → Settings → Security → Personal access tokens, scope `file_content:read` (для `doctor` полезен ещё `current_user:read`).

Вывод — JSON-конверт `{v, command, exit, data, warning, error}`, для человека `--human`. Команды и коды выхода — `figma-kai --help`.
