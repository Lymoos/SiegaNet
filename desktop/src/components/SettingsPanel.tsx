import { useState } from "react";
import { resetControlApi } from "../api";
import { clearActivation, loadActivation } from "../state/activation";
import {
  loadSettings,
  saveSettings,
  type ApiMode,
  type Settings,
} from "../state/settings";
import { isTauri, setAutostart, startCore, stopCore } from "../tauri";

interface Props {
  onClose: () => void;
}

/**
 * Settings slide-over: API mode (mock / real core), path to the core binary,
 * control port, autostart and auto-connect. Values are non-secret and live
 * in localStorage; the control token is never persisted.
 */
export function SettingsPanel({ onClose }: Props) {
  const [s, setS] = useState<Settings>(loadSettings);
  const [coreMsg, setCoreMsg] = useState<string | null>(null);
  const inTauri = isTauri();

  const update = (patch: Partial<Settings>) => {
    const next = { ...s, ...patch };
    setS(next);
    saveSettings(next);
    if ("apiMode" in patch || "controlPort" in patch || "controlToken" in patch) {
      resetControlApi();
    }
  };

  const toggleAutostart = () => {
    const next = !s.autostart;
    update({ autostart: next });
    void setAutostart(next).catch(() => {});
  };

  const runCore = async () => {
    setCoreMsg(null);
    try {
      const msg = await startCore(s.corePath, [
        "-config",
        "client.toml",
        "-control-port",
        String(s.controlPort),
      ]);
      setCoreMsg(msg);
    } catch (e) {
      setCoreMsg(String(e));
    }
  };

  const haltCore = async () => {
    setCoreMsg(null);
    try {
      setCoreMsg(await stopCore());
    } catch (e) {
      setCoreMsg(String(e));
    }
  };

  return (
    <>
      <div className="settings-scrim" onClick={onClose} />
      <div className="settings-panel">
        <div className="settings-title">Настройки</div>

        <div className="set-group">
          <div className="set-label">Источник данных</div>
          <div className="seg">
            {(["mock", "core"] as ApiMode[]).map((m) => (
              <button
                key={m}
                className={s.apiMode === m ? "on" : ""}
                onClick={() => update({ apiMode: m })}
              >
                {m === "mock" ? "Мок (автономно)" : "Ядро (127.0.0.1)"}
              </button>
            ))}
          </div>
          <div className="set-row-sub" style={{ marginTop: 8 }}>
            Мок полностью имитирует контрол-API ядра. Режим «Ядро» — поздний шаг
            интеграции: требует запущенный sieganet-client с контрол-API.
          </div>
        </div>

        <div className="set-group">
          <div className="set-label">Ядро sieganet-client</div>
          <div className="set-field">
            <label>Путь к бинарю ядра</label>
            <input
              placeholder="C:\SiegaNet\sieganet-client.exe"
              value={s.corePath}
              onChange={(e) => update({ corePath: e.target.value })}
            />
          </div>
          <div className="set-field">
            <label>Порт контрол-API (127.0.0.1)</label>
            <input
              value={String(s.controlPort)}
              onChange={(e) => {
                const p = parseInt(e.target.value, 10);
                if (!Number.isNaN(p)) update({ controlPort: p });
              }}
            />
          </div>
          <div className="set-row">
            <button
              className="reconnect-btn"
              style={{ marginTop: 0 }}
              disabled={!inTauri || !s.corePath}
              onClick={runCore}
            >
              Запустить ядро (админ)
            </button>
            <button
              className="reconnect-btn"
              style={{ marginTop: 0, background: "var(--panel-2)", boxShadow: "none" }}
              disabled={!inTauri}
              onClick={haltCore}
            >
              Остановить
            </button>
          </div>
          {coreMsg && <div className="set-row-sub">{coreMsg}</div>}
          {!inTauri && (
            <div className="set-row-sub">
              Управление процессом ядра доступно только в приложении Tauri (в
              браузере — только мок).
            </div>
          )}
        </div>

        <div className="set-group">
          <div className="set-label">Поведение</div>
          <div className="set-row">
            <div>
              <div className="set-row-text">Автозапуск при входе в систему</div>
              <div className="set-row-sub">Приложение стартует свёрнутым в трей</div>
            </div>
            <button
              className={`toggle ${s.autostart ? "on" : ""}`}
              onClick={toggleAutostart}
              aria-label="Автозапуск"
            />
          </div>
          <div className="set-row">
            <div>
              <div className="set-row-text">Автоподключение</div>
              <div className="set-row-sub">К последнему серверу при запуске</div>
            </div>
            <button
              className={`toggle ${s.autoConnect ? "on" : ""}`}
              onClick={() => update({ autoConnect: !s.autoConnect })}
              aria-label="Автоподключение"
            />
          </div>
        </div>

        <div className="set-group">
          <div className="set-label">Подписка</div>
          <div className="set-row">
            <div>
              <div className="set-row-text">Активация</div>
              <div className="set-row-sub">
                {(() => {
                  const a = loadActivation();
                  return a
                    ? `действует до ${new Date(a.validUntil * 1000).toLocaleDateString("ru-RU")}`
                    : "не активировано";
                })()}
              </div>
            </div>
            <button
              className="reconnect-btn"
              style={{ marginTop: 0, width: "auto", background: "var(--panel-2)", boxShadow: "none" }}
              onClick={() => {
                clearActivation();
                window.location.reload();
              }}
            >
              Сбросить
            </button>
          </div>
        </div>

        <div className="settings-note">
          Ядру нужны права администратора: оно создаёт TUN-интерфейс, меняет
          маршруты и правила файрвола. UI не хранит секретов: токен контрол-API
          живёт только в памяти, конфиг пиров — у ядра.
        </div>
      </div>
    </>
  );
}
