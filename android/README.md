# SiegaNet Android

Android-клиент SiegaNet: **Kotlin + Jetpack Compose**, дизайн по утверждённому
макету (большая круглая кнопка с фиолетовым свечением, таймер сессии, выбор
сервера одним тапом). VPN-логики в Kotlin нет: приложение создаёт TUN через
`VpnService` и передаёт fd Go-ядру (gomobile), которое владеет туннелем.

## Сборка

Открыть папку `android/` в Android Studio и запустить `app` — проект
самодостаточен и работает на **мок-ядре** без `.aar`. Из консоли:

```sh
./gradlew :app:assembleDebug
```

## Точка интеграции с gomobile-ядром

Единственный интерфейс — `core/SiegaCore.kt` (только примитивные типы,
совместимо с gomobile):

```kotlin
interface SiegaCore {
    fun start(tunFd: Int, configJson: String)  // fd от VpnService.establish()
    fun stop()
    fun statusJson(): String                    // тот же JSON, что GET /status
}
```

Подключение реального ядра (поздний шаг):

1. `gomobile bind -target=android -o app/libs/siega.aar ./mobile` (из корня репо);
2. раскомментировать `implementation(files("libs/siega.aar"))` в `app/build.gradle.kts`;
3. заменить `CoreRegistry.core` на реализацию поверх `mobile.Mobile.*`;
4. в `SiegaVpnService` переключить `FULL_TUNNEL = true` — тогда в туннель
   уходит `0.0.0.0/0` и DNS. Пока стоит мок, маршрутизируется только
   внутренняя подсеть `10.77.0.0/24`, чтобы телефон не терял интернет при
   «подключённом» моке.

## Что уже работает (на моке)

- Экран подключения: статус сверху («Не подключено» / «Подключение…» /
  «Защищено»), круглая кнопка со свечением и пульсом при подключении,
  таймер сессии и трафик под кнопкой, селектор сервера снизу
  (флаг + страна + пинг + шеврон) с бottom-sheet списком (страна, город, пинг).
- `VpnService` + foreground-уведомление со статусом и действием «Отключить»
  (иначе Android убивает VPN в фоне), обработка `onRevoke`.
- Плитка быстрых настроек (подключить/отключить; без выданного согласия VPN
  открывает приложение — согласие можно запросить только из activity).
- Импорт конфига:
  - deep link `sieganet://import?c=<base64url(конфиг)>`;
  - QR-код с тем же URI (или сырым TOML/JSON конфига) — сканер zxing.
  Конфиг лежит в EncryptedSharedPreferences и передаётся ядру как есть;
  UI секретные поля не разбирает (только `peer_id`/`server` для подписи).

## Дизайн-токены

`ui/theme/Theme.kt` — зеркало `design/tokens.json` (общие с десктопом).
Шрифты забандлены (variable TTF): Outfit (заголовки/кнопки), Inter (текст,
кириллица), JetBrains Mono (пинг/данные/метки).
