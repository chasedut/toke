#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use portable_pty::{native_pty_system, CommandBuilder, PtySize};
use std::io::{Read, Write};
use std::sync::{Arc, Mutex};
use std::thread;
use tauri::{Emitter, Manager};

struct PtyState {
    writer: Option<Box<dyn Write + Send>>,
}

fn find_toke_dev() -> Result<String, String> {
    eprintln!("Current directory: {:?}", std::env::current_dir());
    
    let toke_paths = vec![
        "../../build/toke-darwin-arm64/toke",
        "../build/toke-darwin-arm64/toke",
        "../build/Toke.app/Contents/MacOS/toke",
        "/Users/cd/github/orgs/toke/build/toke-darwin-arm64/toke",
        "/usr/local/bin/toke",
        "toke",
    ];
    
    toke_paths
        .iter()
        .find_map(|p| {
            let path = std::path::Path::new(p);
            if path.exists() {
                // Convert to absolute path for portable-pty
                let abs_path = if path.is_relative() {
                    std::env::current_dir().ok()?.join(path).canonicalize().ok()
                } else {
                    path.canonicalize().ok()
                };
                eprintln!("Found toke at: {:?}", abs_path);
                abs_path.map(|p| p.to_string_lossy().to_string())
            } else {
                eprintln!("Path not found: {}", p);
                None
            }
        })
        .ok_or_else(|| "Could not find toke binary".to_string())
}

#[tauri::command]
fn start_toke(
    app_handle: tauri::AppHandle,
    state: tauri::State<Arc<Mutex<PtyState>>>,
    cols: u16,
    rows: u16,
) -> Result<(), String> {
    let pty_system = native_pty_system();
    
    let pty_pair = pty_system
        .openpty(PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        })
        .map_err(|e| e.to_string())?;

    // Find toke binary - check bundled resource first, then other locations
    let (toke_path, toke_dir) = if let Ok(resource_path) = app_handle.path().resource_dir() {
        // In production, use the bundled toke binary
        // Due to the relative path in resources, it's nested under _up_/_up_/build/
        let bundled_dir = resource_path.join("_up_").join("_up_").join("build").join("toke-darwin-arm64");
        let bundled_toke = bundled_dir.join("toke");
        if bundled_toke.exists() {
            eprintln!("Using bundled toke at: {:?}", bundled_toke);
            (bundled_toke.to_string_lossy().to_string(), Some(bundled_dir))
        } else {
            // Try without the nested path (for future cleaner builds)
            let alt_bundled_dir = resource_path.join("toke-darwin-arm64");
            let alt_bundled_toke = alt_bundled_dir.join("toke");
            if alt_bundled_toke.exists() {
                eprintln!("Using bundled toke at: {:?}", alt_bundled_toke);
                (alt_bundled_toke.to_string_lossy().to_string(), Some(alt_bundled_dir))
            } else {
                // Fallback for development
                (find_toke_dev()?, None)
            }
        }
    } else {
        // Development mode
        (find_toke_dev()?, None)
    };
    
    eprintln!("Using toke binary at: {}", toke_path);

    // Start a shell that runs toke, so when toke exits, user lands in shell
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/bash".to_string());
    let mut cmd = CommandBuilder::new(&shell);
    cmd.env("TERM", "xterm-256color");
    
    // If we have a bundled toke directory, set it as the working directory
    // so toke can find its backends and ngrok
    let use_bundled = toke_dir.is_some();
    if let Some(dir) = toke_dir {
        cmd.cwd(dir);
        eprintln!("Setting working directory for bundled resources");
    }
    
    // Start shell with command to run toke, then clear and reset terminal before continuing
    cmd.arg("-c");
    let toke_cmd = if use_bundled {
        "./toke".to_string()  // Use relative path when in bundled directory
    } else {
        toke_path.clone()     // Use full path otherwise
    };
    cmd.arg(format!(
        "{} || true; printf '\\033[2J\\033[H\\033[?25h'; clear; exec {}",
        toke_cmd, shell
    ));
    
    let mut child = pty_pair
        .slave
        .spawn_command(cmd)
        .map_err(|e| e.to_string())?;

    let mut reader = pty_pair.master.try_clone_reader().map_err(|e| e.to_string())?;
    let writer = pty_pair.master.take_writer().map_err(|e| e.to_string())?;

    // Store writer in state
    {
        let mut state = state.lock().unwrap();
        state.writer = Some(writer);
    }

    // Read output in separate thread
    thread::spawn(move || {
        let mut buf = [0u8; 4096];
        loop {
            match reader.read(&mut buf) {
                Ok(0) => break,
                Ok(n) => {
                    let data = buf[..n].to_vec();
                    let _ = app_handle.emit("pty-output", data);
                }
                Err(_) => break,
            }
        }
        
        // Wait for toke to exit
        let exit_status = child.wait();
        eprintln!("Toke process exited with status: {:?}", exit_status);
    });

    Ok(())
}

#[tauri::command]
fn write_to_pty(state: tauri::State<Arc<Mutex<PtyState>>>, data: String) -> Result<(), String> {
    let mut state = state.lock().unwrap();
    if let Some(writer) = state.writer.as_mut() {
        writer.write_all(data.as_bytes()).map_err(|e| e.to_string())?;
        writer.flush().map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
fn resize_pty(
    _state: tauri::State<Arc<Mutex<PtyState>>>,
    cols: u16,
    rows: u16,
) -> Result<(), String> {
    // TODO: Implement PTY resize
    println!("Resize to {}x{}", cols, rows);
    Ok(())
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_process::init())
        .manage(Arc::new(Mutex::new(PtyState { writer: None })))
        .invoke_handler(tauri::generate_handler![start_toke, write_to_pty, resize_pty])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
