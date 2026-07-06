//! SiegaNet desktop shell (Tauri).
//!
//! The Rust side owns exactly two responsibilities and no VPN logic:
//!   1. the tray icon (status tooltip + quick connect/disconnect/quit), and
//!   2. launching/stopping the Go core `sieganet-client` as an elevated
//!      child process (the core needs admin rights for TUN/routes/firewall).
//!
//! All VPN state comes from the core's control API on 127.0.0.1 and is read
//! by the frontend; the frontend pushes tray updates down via
//! `set_tray_status` and receives tray clicks back as `tray-command` events.

use std::process::{Child, Command};
use std::sync::Mutex;

use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    Emitter, Manager, Runtime, State,
};

/// Handle of the launched core process (or its elevation wrapper).
struct CoreProcess(Mutex<Option<Child>>);

struct TrayMenu<R: Runtime> {
    status_item: MenuItem<R>,
    toggle_item: MenuItem<R>,
}

/// Frontend pushes every VPN state change here so the tray mirrors the UI.
#[tauri::command]
fn set_tray_status(
    app: tauri::AppHandle,
    state: String,
    detail: String,
) -> Result<(), String> {
    let label = match state.as_str() {
        "connected" => {
            if detail.is_empty() {
                "Защищено".to_string()
            } else {
                format!("Защищено — {detail}")
            }
        }
        "connecting" => "Подключение…".to_string(),
        _ => "Не подключено".to_string(),
    };
    let toggle = if state == "disconnected" {
        "Подключиться"
    } else {
        "Отключиться"
    };

    if let Some(menu) = app.try_state::<TrayMenu<tauri::Wry>>() {
        menu.status_item.set_text(&label).map_err(|e| e.to_string())?;
        menu.toggle_item.set_text(toggle).map_err(|e| e.to_string())?;
    }
    if let Some(tray) = app.tray_by_id("main") {
        tray.set_tooltip(Some(format!("SiegaNet — {label}")))
            .map_err(|e| e.to_string())?;
    }
    Ok(())
}

/// Launch the Go core with elevation. The elevation mechanism is per-OS:
/// Windows — UAC via PowerShell `Start-Process -Verb RunAs`;
/// Linux — pkexec; macOS — osascript administrator prompt.
///
/// The UI treats this as opaque: success here only means "the launcher was
/// spawned"; the real health signal is the control API answering on
/// 127.0.0.1 afterwards.
#[tauri::command]
fn core_start(
    proc: State<CoreProcess>,
    path: String,
    args: Vec<String>,
) -> Result<String, String> {
    if path.trim().is_empty() {
        return Err("путь к бинарю ядра не задан".into());
    }
    let mut guard = proc.0.lock().map_err(|e| e.to_string())?;
    if guard.is_some() {
        return Err("ядро уже запущено этим приложением".into());
    }

    let child = spawn_elevated(&path, &args).map_err(|e| e.to_string())?;
    *guard = Some(child);
    Ok("ядро запускается (проверьте запрос прав администратора)".into())
}

#[tauri::command]
fn core_stop(proc: State<CoreProcess>) -> Result<String, String> {
    let mut guard = proc.0.lock().map_err(|e| e.to_string())?;
    match guard.take() {
        Some(mut child) => {
            // Best effort: on Windows the elevated process outlives the UAC
            // wrapper we hold, so ask the OS to terminate it by image name.
            let _ = child.kill();
            #[cfg(target_os = "windows")]
            {
                let _ = Command::new("powershell")
                    .args([
                        "-NoProfile",
                        "-Command",
                        "Start-Process taskkill -ArgumentList '/IM','sieganet-client.exe','/F' -Verb RunAs -WindowStyle Hidden",
                    ])
                    .spawn();
            }
            Ok("ядро остановлено".into())
        }
        None => Err("ядро не запускалось этим приложением".into()),
    }
}

#[cfg(target_os = "windows")]
fn spawn_elevated(path: &str, args: &[String]) -> std::io::Result<Child> {
    // -Verb RunAs triggers the UAC prompt; the core itself declares
    // requireAdministrator in its manifest, so this is belt-and-braces.
    let arg_list = args
        .iter()
        .map(|a| format!("'{}'", a.replace('\'', "''")))
        .collect::<Vec<_>>()
        .join(",");
    let ps = if args.is_empty() {
        format!(
            "Start-Process -FilePath '{}' -Verb RunAs -WindowStyle Hidden",
            path.replace('\'', "''")
        )
    } else {
        format!(
            "Start-Process -FilePath '{}' -ArgumentList {} -Verb RunAs -WindowStyle Hidden",
            path.replace('\'', "''"),
            arg_list
        )
    };
    Command::new("powershell")
        .args(["-NoProfile", "-Command", &ps])
        .spawn()
}

#[cfg(target_os = "linux")]
fn spawn_elevated(path: &str, args: &[String]) -> std::io::Result<Child> {
    let mut cmd = Command::new("pkexec");
    cmd.arg(path).args(args);
    cmd.spawn()
}

#[cfg(target_os = "macos")]
fn spawn_elevated(path: &str, args: &[String]) -> std::io::Result<Child> {
    let joined = std::iter::once(path.to_string())
        .chain(args.iter().cloned())
        .map(|a| format!("'{}'", a.replace('\'', r"'\''")))
        .collect::<Vec<_>>()
        .join(" ");
    Command::new("osascript")
        .args([
            "-e",
            &format!("do shell script \"{joined}\" with administrator privileges"),
        ])
        .spawn()
}

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            None,
        ))
        .manage(CoreProcess(Mutex::new(None)))
        .setup(|app| {
            // --- tray ---------------------------------------------------
            let status_item =
                MenuItem::with_id(app, "status", "Не подключено", false, None::<&str>)?;
            let toggle_item =
                MenuItem::with_id(app, "toggle", "Подключиться", true, None::<&str>)?;
            let show_item = MenuItem::with_id(app, "show", "Открыть SiegaNet", true, None::<&str>)?;
            let quit_item = MenuItem::with_id(app, "quit", "Выход", true, None::<&str>)?;
            let menu = Menu::with_items(
                app,
                &[&status_item, &toggle_item, &show_item, &quit_item],
            )?;

            TrayIconBuilder::with_id("main")
                .icon(app.default_window_icon().unwrap().clone())
                .tooltip("SiegaNet — Не подключено")
                .menu(&menu)
                .show_menu_on_left_click(true)
                .on_menu_event(|app, event| match event.id.as_ref() {
                    "toggle" => {
                        let _ = app.emit("tray-command", "toggle");
                    }
                    "show" => {
                        if let Some(win) = app.get_webview_window("main") {
                            let _ = win.show();
                            let _ = win.set_focus();
                        }
                    }
                    "quit" => app.exit(0),
                    _ => {}
                })
                .build(app)?;

            app.manage(TrayMenu {
                status_item,
                toggle_item,
            });
            Ok(())
        })
        // closing the window hides to tray — the VPN keeps running
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                let _ = window.hide();
                api.prevent_close();
            }
        })
        .invoke_handler(tauri::generate_handler![
            set_tray_status,
            core_start,
            core_stop
        ])
        .run(tauri::generate_context!())
        .expect("error while running SiegaNet desktop");
}
