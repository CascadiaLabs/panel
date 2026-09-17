# Cascadia Panel

Управляющая панель каскадными нодами Cascadia: регистрация нод, графовый редактор
каскадов (inbound/outbound/rule/balancer), генерация конфигов sing-box и пуш
по gRPC.

## Установка

```sh
sudo bash -c "$(curl -sL https://github.com/CascadiaLabs/install/raw/main/panel.sh)" @ install
```

Поднимает Docker-контейнер на :2083. Логин `admin`, пароль печатается в лог
при первом старте (`docker logs cascadia-panel 2>&1 | grep 'FIRST LOGIN'`) или
задаётся `PANEL_ADMIN_PASSWORD` в `.env` до первого запуска.

Переменные окружения — см. [.env.example](.env.example).

## Почему Cascadia

- **Работает на «серверах-картошках».** Панель — один статичный Go-бинарник со
  встроенным фронтендом (никаких node-процессов в runtime); нода — сборка
  sing-box. Реальный замер на VPS 512MB RAM / 5GB диска / 1 vCPU:

  ```
  NAME    CPU %   MEM USAGE
  panel   0.7%    19.96MiB
  node    0.6%    6.297MiB
  ```

  Вся связка panel+node — **~26MiB RAM**; CPU простаивает и лишь изредка
  всплескивает до 0.7%.

  ```
  IMAGE                             DISK USAGE
  ghcr.io/cascadialabs/panel:latest 11.5MB
  ghcr.io/cascadialabs/node:latest  18.1MB
  ```

  Всё вместе занимает ~30MB на диске — такие деплои помещаются на самый слабый
  VPS, который обычно идёт в нагрузку к чему-нибудь, а не как выделенный хост.
- **sing-box.** Не Xray и не legacy-протоколы: конфиги генерируются нативно под
  современный sing-box (vless+reality, vmess, trojan, shadowsocks, hysteria2,
  tuic), нода — это и есть sing-box с конфигом, а не обвязка сверху.
- **Каскадные сети в UI — единственное решение такого рода.** Цепочки серверов
  (вход в одной стране → выход в другой, balancer'ы, правил) строятся визуально
  на графе: драг-н-дроп элементов канваса, соединения между нодами на разных
  серверах, валидация (зацикливание, несовместимость протоколов, терминальность),
  генерация конфигов и деплой по gRPC на все задействованные ноды — одним
  нажатием.

## Релизы и версионирование

Версии — короткие, без semver-строк:

- `1a`, `1b` — альфа, бета релизы.
- `1`, `2`, `3` — стабильные версии.

Имя git-тега = имя Docker-тега: пуш тега `1a` публикует
`ghcr.io/cascadialabs/panel:1a`; `:latest` всегда указывает на `main`. Версия
вшита в бинарник и печатается при старте:
`docker logs cascadia-panel 2>&1 | head -1` → `Cascadia Panel 1a`.

Поставить конкретную версию:

```sh
sudo bash -c "PANEL_IMAGE=ghcr.io/cascadialabs/panel:1a $(curl -sL https://github.com/CascadiaLabs/install/raw/main/panel.sh) @ install"
```

## Возможности

- **Аутентификация**: вход по логину/паролю (admin, bcrypt, HttpOnly-cookie
  сессии) + `PANEL_TOKEN` как machine-to-machine bearer-ключ для скриптов.
- **Ноды**: CRUD, статус по gRPC (TLS с пиннингом сертификата), ручной пуш
  конфига.
- **Графы каскадов**: канвас (svelte-flow) с доками Клиент/Интернет;
  элементы inbound/outbound/rule/balancer; соединения между нодами разных
  серверов; валидация (матрица соединений, зацикливание, соответствие
  протоколов, терминальность); генерация нативных конфигов sing-box и
  деплой по gRPC с отчётом по каждой ноде.
- «Balancer» в UI = `urltest` (авто-выбор по меньшему пингу) или `selector`
  из sing-box — balancer'ов как в Xray в sing-box нет.

Протоколы: vless (+reality), vmess, trojan, shadowsocks, hysteria2, tuic.

## Разработка

```sh
cd web && npm install && npm run build   # фронтенд → api/static (embed)
go run ./cmd/panel                        # панель на :2083
go test ./...                             # тесты
```

[dev](https://github.com/CascadiaLabs/dev) Локальный стенд с двумя TLS-нодами: `make dev-up` из корня репозитория
(см. `dev/README.md`). Вход в UI: admin; для dev-скриптов — `PANEL_TOKEN`
из `dev/.state/panel.env`.
