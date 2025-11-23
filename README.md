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
go run ./cmd/p2p-chat -userName=Vasya
```

Параметры:

- `-topicName` — имя топика (по умолчанию: `my_applesauce`)
- `-userName` — имя пользователя (по умолчанию: USER-<короткий peer id>)
- `-peer` — multiaddr пира для прямого подключения; можно указать несколько раз
- `-relay` — multiaddr relay v2 пира для AutoRelay; можно указать несколько раз
- `-noDHT` — отключить поиск пиров через Kademlia DHT
- `--db-path` — путь к SQLite БД для истории сообщений и настроек

При старте нода печатает свой `Peer ID` и список адресов. Любой адрес вида `/ip4/.../tcp/.../p2p/...` можно передать другой ноде через `-peer`.

## Локальная проверка

В первом терминале:

```sh
go run ./cmd/p2p-chat -noDHT -topicName=local -userName=alice
```

Во втором терминале используйте один из адресов первой ноды:

```sh
go run ./cmd/p2p-chat -noDHT -topicName=local -userName=bob -peer /ip4/127.0.0.1/tcp/PORT/p2p/PEER_ID
```

После подключения сообщения, введенные в одном терминале, должны появляться в другом. На одной машине и в LAN также включен mDNS, поэтому локальные ноды часто находят друг друга автоматически.

## SQLite

SQLite включается явно:

```sh
go run ./cmd/p2p-chat -topicName=local -userName=alice --db-path ./p2p-chat.db
```

При старте применяются миграции, загружается последняя история комнаты, входящие и исходящие сообщения сохраняются идемпотентно по `message.id`, а настройки `last_room` и `last_username` пишутся в таблицу `settings`. Драйвер SQLite — `modernc.org/sqlite`, CGO не нужен.

## NAT traversal

Включены доступные механизмы libp2p:

- NAT port mapping через UPnP/NAT-PMP (`NATPortMap`)
- AutoNAT v2 и AutoNAT service
- relay transport и circuit relay v2 service
- DCUtR/hole punching
- AutoRelay при наличии статических relay-кандидатов через `-relay`

## Структура

- `cmd/p2p-chat` — CLI entrypoint, флаги и graceful shutdown по Ctrl+C/SIGTERM
- `internal/chat` — основная логика libp2p host, discovery, pubsub и сообщений
