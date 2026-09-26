// figma-kai — макеты Figma из терминала и из агента: картинка экрана по
// ссылке из задачи и компактное дерево слоёв с размерами, цветами и текстом.
//
// Точка входа тонкая: вся логика — в пакетах internal/. Карта:
//
//	internal/command   CLI-слой: роутер Run, usage, разбор флагов, все команды
//	internal/figma     клиент REST API, разбор ссылок, сжатие дерева слоёв
//	internal/config    файл по умолчанию, каталог картинок, токен
//	internal/output    единый JSON-конверт и человекочитаемый режим
//	internal/exit      коды выхода как контракт с вызывающим
package main

import (
	"os"

	"github.com/Kaidstor/figma-kai/internal/command"
)

func main() { os.Exit(command.Run(os.Args[1:])) }
