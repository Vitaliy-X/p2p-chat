# P2P-chat на Go с использованием libp2p

Этот проект реализует простой децентрализованный чат на Go с помощью библиотеки [libp2p](https://github.com/libp2p/go-libp2p). Каждый участник может подключаться к другим пирами, обмениваться сообщениями в реальном времени и находить других участников с помощью Kademlia DHT.

## Установка

1. Клонируйте репозиторий:

```sh
    git clone https://github.com/yourusername/p2p-chat-go.git
    cd p2p-chat-go
```

2. Установите зависимости:

```sh
    go mod tidy
```

## Запуск

Запустите чат с указанием имени пользователя:

```sh
go run ./cmd/p2p-chat -userName=Vasya -room-key shared-secret
```

Параметры:

- `-topicName` — имя топика (по умолчанию: `my_applesauce`)
- `-room-key` — общий ключ закрытой комнаты; можно передать через `P2P_CHAT_ROOM_KEY`
- `-userName` — имя пользователя (по умолчанию: USER-<короткий peer id>)
- `-peer` — multiaddr пира для прямого подключения; можно указать несколько раз
- `-relay` — multiaddr relay v2 пира для AutoRelay; можно указать несколько раз
- `-noDHT` — отключить поиск пиров через Kademlia DHT
- `--db-path` — путь к SQLite БД для истории сообщений и настроек

При старте нода печатает свой `Peer ID` и список адресов. Любой адрес вида `/ip4/.../tcp/.../p2p/...` можно передать другой ноде через `-peer`.

## Desktop GUI

Минимальный desktop-клиент сделан на Wails + Svelte + TypeScript:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
cd cmd/p2p-chat-gui
wails dev
```

Сборка desktop-приложения:

```sh
cd cmd/p2p-chat-gui
wails build
```

GUI использует те же libp2p/SQLite-слои, что и CLI: можно выбрать username, room, room key, SQLite path, включить DHT discovery, подключиться вручную по multiaddr, скопировать peer info, отправлять сообщения и видеть историю комнаты.

## Локальная проверка

В первом терминале:

```sh
go run ./cmd/p2p-chat -noDHT -topicName=local -room-key shared-secret -userName=alice
```

Во втором терминале используйте один из адресов первой ноды:

```sh
go run ./cmd/p2p-chat -noDHT -topicName=local -room-key shared-secret -userName=bob -peer /ip4/127.0.0.1/tcp/PORT/p2p/PEER_ID
```

После подключения сообщения, введенные в одном терминале, должны появляться в другом. На одной машине и в LAN также включен mDNS, поэтому локальные ноды часто находят друг друга автоматически.

## SQLite

SQLite включается явно:

```sh
go run ./cmd/p2p-chat -topicName=local -room-key shared-secret -userName=alice --db-path ./p2p-chat.db
```

При старте применяются миграции goose из `internal/store/sqlite/migrations`, загружается последняя история комнаты, входящие и исходящие сообщения сохраняются идемпотентно по `message.id`, а настройки `last_room` и `last_username` пишутся в таблицу `settings`. Драйвер SQLite — `modernc.org/sqlite`, CGO не нужен.

SQLite-схема разделяет комнаты и сообщения: `rooms` хранит комнаты и salt, `messages` хранит метаданные сообщения и зашифрованный payload. Текст сообщения и `sender_username` шифруются через AES-256-GCM ключом, выведенным из `room-key`, имени комнаты и per-room salt через Argon2id. При переходе со старой plaintext-схемы старые сообщения не переносятся, потому что миграция не знает `room-key`.

## Закрытые комнаты

Комнаты закрываются общим `room-key`: сетевой pubsub topic, DHT discovery namespace и mDNS service name вычисляются через HMAC от имени комнаты и ключа. Клиент, который знает только `room=local`, но не знает ключ, не попадет в тот же topic и не увидит сообщения.

Сообщения в pubsub шифруются через AES-256-GCM, ключ шифрования выводится из `room-key` и имени комнаты. `room-key` не сохраняется в SQLite settings. Используйте длинный случайный ключ для реальных комнат.

SQLite-история хранит зашифрованный payload сообщений. Для индексов и загрузки истории в БД остаются plaintext-метаданные: `message.id`, room metadata, `sender_id`, `sent_at`, `version`, `received_at`.

## NAT traversal

Включены доступные механизмы libp2p:

- NAT port mapping через UPnP/NAT-PMP (`NATPortMap`)
- AutoNAT v2 и AutoNAT service
- relay transport и circuit relay v2 service
- DCUtR/hole punching
- AutoRelay при наличии статических relay-кандидатов через `-relay`

## Структура

- `cmd/p2p-chat` — CLI entrypoint, флаги и graceful shutdown по Ctrl+C/SIGTERM
- `cmd/p2p-chat-gui` — Wails desktop entrypoint и backend-адаптер
- `cmd/p2p-chat-gui/frontend` — Svelte + TypeScript интерфейс
- `internal/chat` — основная логика libp2p host, discovery, pubsub и сообщений
- `internal/store/sqlite` — SQLite-адаптер хранения через `database/sql`
- `internal/store/sqlite/migrations` — SQL-миграции goose, встроенные в бинарник
