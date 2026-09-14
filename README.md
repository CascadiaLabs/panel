# Cascadia Panel

Управляющая панель каскадными нодами Cascadia: регистрация нод, графовый редактор
каскадов (inbound/outbound/rule/balancer), генерация конфигов sing-box и пуш
по gRPC.

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

Протоколы: vless (+reality), vmess, trojan, shadowsocks, hysteria2, tuic,
direct.

## Установка

```sh
sudo bash -c "$(curl -sL https://github.com/CascadiaLabs/install/raw/main/panel.sh)" @ install
```

Поднимает Docker-контейнер на :2083. Логин `admin`, пароль печатается в лог
при первом старте (`docker logs panel 2>&1 | grep 'FIRST LOGIN'`) или
задаётся `PANEL_ADMIN_PASSWORD` в `.env` до первого запуска.

Переменные окружения — см. [.env.example](.env.example).

## Разработка

```sh
cd web && npm install && npm run build   # фронтенд → api/static (embed)
go run ./cmd/panel                        # панель на :2083
go test ./...                             # тесты
```

Локальный стенд с двумя TLS-нодами: `make dev-up` из корня репозитория
(см. `dev/README.md`). Вход в UI: admin; для dev-скриптов — `PANEL_TOKEN`
из `dev/.state/panel.env`.
