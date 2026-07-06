# SiegaNet Desktop

Десктоп-клиент SiegaNet: **Tauri 2 + React + TypeScript**. Это оболочка над
Go-ядром `sieganet-client` — UI не содержит VPN-логики. Он запускает ядро,
показывает статус и отдаёт команды по локальному контрол-API.

## Контракт с ядром

UI построен строго под контрол-API ядра на `127.0.0.1:<port>` (порт из
конфига, авторизация локальным токеном):

```
GET  /status      -> { state, server_id, inner_ip, since_unix, up_bytes, down_bytes }
GET  /servers     -> [ { id, country, city, host, ping_ms, load_pct } ]
POST /connect     { server_id }   -> { ok }
POST /disconnect                  -> { ok }
POST /activate    { key }         -> { ok, valid_until }
```

**Модель подписки:** без валидного ключа активации клиент не работает —
при старте показывается экран активации. Ключ проверяется через
`POST /activate` (сейчас мок: любой непустой ключ валиден 30 дней; реальная
проверка — бэкенд, тот же формат) и кешируется вместе со сроком действия,
поэтому ключ не спрашивается при каждом запуске. Сбросить активацию можно
в настройках.

Пока ядро не отдаёт этот API, UI работает на **моке** (`src/api/mock.ts`),
который реализует тот же интерфейс `ControlApi` с живым поведением
(handshake-фаза `connecting`, накопление трафика, дрожание пингов).
Переключатель «мок / ядро» — в настройках приложения; HTTP-клиент реального
ядра уже написан (`src/api/http.ts`) — интеграция сводится к появлению API на
стороне ядра.

## Запуск

Только UI (браузер, мок, без Rust-тулчейна):

```sh
npm install
npm run dev        # http://localhost:5173
```

Полное приложение (окно, трей, запуск ядра):

```sh
# требования: Rust stable; на Linux — webkit2gtk-4.1 + gtk3 dev-пакеты,
# на Windows — WebView2 (стоит по умолчанию), MSVC Build Tools
npm install
npm run tauri dev    # dev-окно
npm run tauri build  # установщик/бандл
```

Иконки уже сгенерированы (`src-tauri/icons/`); пересоздать:
`node scripts/gen-icon.mjs && npx tauri icon icon-source.png`.

## Структура

```
src/
  api/        контракт (types.ts), мок (mock.ts), http-клиент ядра (http.ts)
  state/      настройки (localStorage, без секретов) + useVpn — поллинг статуса
  components/ StatusBar, Sidebar (последнее подключение + список серверов),
              WorldMap (react-simple-maps + Natural Earth), ServerCard, SettingsPanel
  geo/        город -> [lon, lat] для точек на карте
  tauri.ts    guarded-мост к Tauri (трей, запуск ядра) — в браузере no-op
src-tauri/
  src/lib.rs  трей (статус/подключить/открыть/выход), запуск sieganet-client
              с повышением прав (UAC / pkexec / osascript), скрытие в трей
```

## Принципы

- **Никакой VPN-логики в UI**: туннель, QUIC, шифрование, TUN — только в ядре.
- Ядро запускается **с правами администратора** (TUN/маршруты/файрвол).
- Секреты не хранятся: токен контрол-API живёт в памяти, в localStorage —
  только несекретные настройки (путь к ядру, порт, тумблеры, последний сервер).
- Закрытие окна сворачивает в трей; VPN продолжает работать.
