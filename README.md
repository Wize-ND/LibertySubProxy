# LibertySubProxy

Лёгкий реверс-прокси для роутера **Keenetic Hopper SE** (aarch64).
Слушает `:65432`, GET `/roskom-whocares/{sub}` пересылает на
`https://connliberty.com/connection/subs/{sub}` и транслирует ответ обратно
(статус, заголовки, тело).

## Сборка (Windows, кросс-компиляция)

```cmd
build\build.bat
```

Получим:
- `bin\libertysubproxy-arm64` — статический ELF-бинарник для Keenetic Hopper SE
- `bin\libertysubproxy-armv7` — на всякий случай для 32-битных Keenetic
- `bin\debug\libertysubproxy.exe` — Windows-версия для локальной отладки
  (намеренно в отдельной папке `debug\`, чтобы не перепутать при копировании на роутер)

Бинарник собирается с `CGO_ENABLED=0 -ldflags "-s -w" -trimpath` —
без зависимостей от libc, размер ~3-4 МБ.

## Установка на Keenetic (Entware/opkg)

1. Убедиться, что установлен Entware (OPKG) через веб-интерфейс Keenetic.
2. Скопировать файлы на роутер:

```sh
scp bin/libertysubproxy-arm64 root@192.168.1.1:/opt/bin/libertysubproxy
scp deploy/S99libertysubproxy root@192.168.1.1:/opt/etc/init.d/S99libertysubproxy
```

3. На роутере:

```sh
chmod +x /opt/bin/libertysubproxy
chmod +x /opt/etc/init.d/S99libertysubproxy
mkdir -p /opt/var/run /opt/var/log
/opt/etc/init.d/S99libertysubproxy start
```

Проверка:

```sh
/opt/etc/init.d/S99libertysubproxy status
curl "http://127.0.0.1:65432/health"
curl "http://127.0.0.1:65432/roskom-whocares/test"
tail /opt/var/log/libertysubproxy.log
```


Автозапуск при старте роутера работает сам: Entware выполняет все
`/opt/etc/init.d/S*` скрипты при загрузке.

## Флаги

| Флаг         | По умолчанию                        | Описание                             |
|--------------|-------------------------------------|--------------------------------------|
| `-listen`    | `:65432`                            | Адрес прослушивания `host:port`      |
| `-upstream`  | `https://connliberty.com/connection/subs/` | Базовый URL upstream          |
| `-d`         | off                                 | Демонизироваться (setsid + pidfile)  |
| `-pidfile`   | `/opt/var/run/libertysubproxy.pid`  | Путь к pid-файлу                     |
| `-memlimit`  | `16`                                | Софт-лимит кучи Go (GOMEMLIMIT), MiB |
| `-version`   | —                                   | Печатать версию и выйти              |

## Минимизация памяти

- `GOMEMLIMIT=16MiB` + `GOGC=50` — GC агрессивнее освобождает кучу,
  RSS держится в пределах 8–20 МБ.
- `GOMAXPROCS=2` максимум — меньше служебных структур планировщика.
- Транспорт: `MaxIdleConns=2`, `MaxConnsPerHost=4`, HTTP/2 отключён.
- Стриминг тела через `io.Copy` — ответ не буферизуется в памяти целиком.
- Статическая сборка без cgo — нет динамического линкера и его накладных расходов.

## Эндпоинты

- `GET /roskom-whocares/{sub}` — прокси на upstream
- `GET /health` — `200 ok` для мониторинга

Graceful shutdown по `SIGTERM`/`SIGINT` (до 5 сек на доработку активных запросов).

## Как открыть порт наружу

Чтобы открыть порт, вам обязательно нужен публичный (белый) IP-адрес от провайдера.

- Откройте браузер, зайдите в панель управления Keenetic (обычно 192.168.1.1).
- Перейдите в меню «Сетевые правила» → «Переадресация портов».Нажмите кнопку «Добавить правило».
- Заполните поля:
  - Статус: Включено.
  - Интерфейс: Выберите ваше основное подключение к интернету (например, Провайдер или PPPoE/L2TP).
  - Протокол: TCP.
  - Тип переадресации: «На этот роутер» (поскольку приложение крутится прямо на самом Keenetic.
  - Номер порта: Укажите порт, который вы хотите открывать для внешнего мира (например, 65432).
  - Перенаправить на порт: 65432.
- Нажмите «Сохранить».