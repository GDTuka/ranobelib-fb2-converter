# RanobeLib → FB2 converter

Конвертер получает список и содержимое глав через публичный API RanobeLib/NovelsLib, загружает иллюстрации и собирает один FB2-файл. Chromium и JWT для публичных книг не требуются.

## Требования

- Go 1.23 или новее.
- Сетевой доступ к `api.novelslib.me` и зеркалу изображений `v1.novelslib.me`.

## Структура

- `cmd/ranobelib-fb2-converter` — CLI.
- `internal/config` — загрузка и проверка конфигурации.
- `internal/converter` — API-клиент, преобразование глав и загрузка изображений.
- `internal/fb2` — построение FB2.
- `output` — локальные результаты конвертации; FB2-файлы исключены из Git.

## Конфигурация

`config.json` уже настроен на книгу «Приговорённый быть героем»:

```json
{
  "bookName": "Приговорённый быть героем",
  "url": "https://ranobelib.me/ru/book/218522--yuusha-kei-ni-shosu-choubatsu-yuusha-9004-tai-keimu-kiroku",
  "apiUrl": "https://api.novelslib.me/api/manga/218522--yuusha-kei-ni-shosu-choubatsu-yuusha-9004-tai-keimu-kiroku",
  "outputDir": "./output"
}
```

Для другой книги замените название и оба адреса. Все поля обязательны; `url` и `apiUrl` должны быть абсолютными HTTP(S)-адресами.

## Запуск

```bash
go run ./cmd/ranobelib-fb2-converter -config ./config.json
```

Конвертер ограничивает частоту запросов к API и повторяет временные сетевые ошибки (`429` и `5xx`). Выходной файл заменяется атомарно только после успешной загрузки и преобразования всех глав, поэтому ошибка не уничтожает уже существующий FB2.

Для текущей конфигурации результат записывается в:

```text
output/Приговорённый быть героем.fb2
```

## Проверка

```bash
go test ./... -race
```

Чтобы дополнительно разобрать реальный созданный файл как XML и проверить число секций:

```bash
FB2_INTEGRATION_FILE="$PWD/output/Приговорённый быть героем.fb2" \
FB2_EXPECTED_SECTIONS=190 \
go test ./internal/fb2 -run TestGeneratedBookFile -v
```
