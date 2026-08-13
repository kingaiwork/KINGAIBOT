use reqwest::{redirect::Policy, Client, Method};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::{net::IpAddr, str::FromStr, sync::Mutex, time::Duration};
use tauri::Manager;
use url::Url;

const MAX_RESPONSE_BYTES: usize = 4 << 20;
const MAX_TOKEN_BYTES: usize = 4096;

#[derive(Clone)]
struct Session {
    base_url: Url,
    token: String,
    client: Client,
}

#[derive(Default)]
struct AppState {
    session: Mutex<Option<Session>>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ConnectArgs {
    server_url: String,
    token: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PairDeviceArgs {
    server_url: String,
    pairing_id: String,
    pairing_secret: String,
    device_name: String,
    platform: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct CreatePairingArgs {
    scopes: Vec<String>,
    expires_in_seconds: u64,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ServerSummary {
    name: String,
    version: String,
    base_url: String,
    ready: bool,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct PairDeviceResult {
    summary: ServerSummary,
    device_token: String,
    device: Value,
}

fn normalize_server_url(raw: &str) -> Result<Url, String> {
    let mut url = Url::parse(raw.trim()).map_err(|e| format!("invalid server URL: {e}"))?;
    if url.cannot_be_a_base() {
        return Err("server URL must be an absolute hierarchical URL".into());
    }
    if !url.username().is_empty() || url.password().is_some() {
        return Err("credentials are not allowed inside the server URL".into());
    }
    if url.query().is_some() || url.fragment().is_some() {
        return Err("server URL must not contain query parameters or fragments".into());
    }
    if url.path() != "/" && !url.path().is_empty() {
        return Err("server URL must point to the server origin, without an API path".into());
    }

    let host = url
        .host_str()
        .ok_or_else(|| "server URL requires a hostname".to_string())?;
    match url.scheme() {
        "https" => {}
        "http" if is_loopback_host(host) => {}
        "http" => return Err("remote KINGAIBOT servers require HTTPS".into()),
        _ => return Err("only HTTPS is allowed; loopback development may use HTTP".into()),
    }

    url.set_path("/");
    Ok(url)
}

fn is_loopback_host(host: &str) -> bool {
    host.eq_ignore_ascii_case("localhost")
        || IpAddr::from_str(host)
            .map(|ip| ip.is_loopback())
            .unwrap_or(false)
}

fn endpoint(base: &Url, segments: &[&str]) -> Result<Url, String> {
    let mut url = base.clone();
    {
        let mut path = url
            .path_segments_mut()
            .map_err(|_| "server URL cannot be used as an API base".to_string())?;
        path.clear();
        for segment in segments {
            path.push(segment);
        }
    }
    Ok(url)
}

fn current_session(state: &tauri::State<'_, AppState>) -> Result<Session, String> {
    state
        .session
        .lock()
        .map_err(|_| "client session lock is poisoned".to_string())?
        .clone()
        .ok_or_else(|| "not connected to a KINGAIBOT server".to_string())
}

fn validate_id(id: &str) -> Result<(), String> {
    let valid = !id.is_empty()
        && id.len() <= 160
        && id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'_' | b'-' | b'.'))
        && id != "."
        && id != "..";
    if valid {
        Ok(())
    } else {
        Err("invalid resource identifier".into())
    }
}

fn validate_token(token: &str) -> Result<(), String> {
    if token.len() < 32 || token.len() > MAX_TOKEN_BYTES || token.chars().any(char::is_whitespace) {
        return Err("access token must be 32-4096 non-whitespace characters".into());
    }
    Ok(())
}

fn secure_client() -> Result<Client, String> {
    Client::builder()
        .redirect(Policy::none())
        .connect_timeout(Duration::from_secs(10))
        .timeout(Duration::from_secs(30))
        .user_agent("KINGAIBOT-Control-Center/0.2")
        .build()
        .map_err(|e| format!("failed to initialize secure HTTP client: {e}"))
}

async fn read_response_limited(
    mut response: reqwest::Response,
) -> Result<(reqwest::StatusCode, Vec<u8>), String> {
    let status = response.status();
    if response.content_length().unwrap_or(0) > MAX_RESPONSE_BYTES as u64 {
        return Err("server response exceeds 4 MiB limit".into());
    }
    let mut out = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|e| format!("response read failed: {e}"))?
    {
        if out.len().saturating_add(chunk.len()) > MAX_RESPONSE_BYTES {
            return Err("server response exceeds 4 MiB limit".into());
        }
        out.extend_from_slice(&chunk);
    }
    Ok((status, out))
}

async fn api_json(
    session: &Session,
    method: Method,
    segments: &[&str],
    body: Option<Value>,
) -> Result<Value, String> {
    let url = endpoint(&session.base_url, segments)?;
    let mut request = session
        .client
        .request(method, url)
        .bearer_auth(&session.token)
        .header("Accept", "application/json");
    if let Some(body) = body {
        request = request.json(&body);
    }
    let response = request
        .send()
        .await
        .map_err(|e| format!("request failed: {e}"))?;
    let (status, bytes) = read_response_limited(response).await?;
    if !status.is_success() {
        let message = String::from_utf8_lossy(&bytes);
        let short = message.chars().take(800).collect::<String>();
        return Err(format!(
            "server returned HTTP {}: {}",
            status.as_u16(),
            short
        ));
    }
    serde_json::from_slice(&bytes).map_err(|e| format!("invalid JSON from server: {e}"))
}

async fn ready(session: &Session) -> Result<bool, String> {
    let url = endpoint(&session.base_url, &["readyz"])?;
    let response = session
        .client
        .get(url)
        .bearer_auth(&session.token)
        .send()
        .await
        .map_err(|e| format!("readiness request failed: {e}"))?;
    Ok(response.status().is_success())
}

async fn summary(session: &Session) -> Result<ServerSummary, String> {
    let health = api_json(session, Method::GET, &["healthz"], None).await?;
    Ok(ServerSummary {
        name: health
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or("KINGAIBOT")
            .to_string(),
        version: health
            .get("version")
            .and_then(Value::as_str)
            .unwrap_or("unknown")
            .to_string(),
        base_url: session.base_url.as_str().trim_end_matches('/').to_string(),
        ready: ready(session).await.unwrap_or(false),
    })
}

async fn authenticate_session(
    base_url: Url,
    token: String,
    state: &tauri::State<'_, AppState>,
) -> Result<ServerSummary, String> {
    validate_token(&token)?;
    let session = Session {
        base_url,
        token,
        client: secure_client()?,
    };
    api_json(&session, Method::GET, &["v1", "tasks"], None).await?;
    let info = summary(&session).await?;
    *state
        .session
        .lock()
        .map_err(|_| "client session lock is poisoned".to_string())? = Some(session);
    Ok(info)
}

#[tauri::command]
async fn connect_server(
    args: ConnectArgs,
    state: tauri::State<'_, AppState>,
) -> Result<ServerSummary, String> {
    let base_url = normalize_server_url(&args.server_url)?;
    authenticate_session(base_url, args.token.trim().to_string(), &state).await
}

#[tauri::command]
async fn pair_device(
    args: PairDeviceArgs,
    state: tauri::State<'_, AppState>,
) -> Result<PairDeviceResult, String> {
    let base_url = normalize_server_url(&args.server_url)?;
    validate_id(args.pairing_id.trim())?;
    let pairing_secret = args.pairing_secret.trim();
    if pairing_secret.len() < 32
        || pairing_secret.len() > 256
        || pairing_secret.chars().any(char::is_whitespace)
    {
        return Err("invalid pairing secret".into());
    }
    let device_name = args.device_name.trim();
    if device_name.is_empty() || device_name.len() > 80 {
        return Err("device name must contain 1-80 characters".into());
    }
    let platform = args
        .platform
        .as_deref()
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .unwrap_or(std::env::consts::OS);
    if platform.len() > 80 {
        return Err("platform label is too long".into());
    }

    let client = secure_client()?;
    let response = client
        .post(endpoint(&base_url, &["v1", "device-pair"])?)
        .header("Accept", "application/json")
        .json(&json!({
            "pairing_id": args.pairing_id.trim(),
            "pairing_secret": pairing_secret,
            "device_name": device_name,
            "platform": platform
        }))
        .send()
        .await
        .map_err(|e| format!("pairing request failed: {e}"))?;
    let (status, bytes) = read_response_limited(response).await?;
    if !status.is_success() {
        return Err(format!("pairing rejected with HTTP {}", status.as_u16()));
    }
    let body: Value =
        serde_json::from_slice(&bytes).map_err(|e| format!("invalid pairing response: {e}"))?;
    let device_token = body
        .get("device_token")
        .and_then(Value::as_str)
        .ok_or_else(|| "pairing response did not contain a device token".to_string())?
        .to_string();
    validate_token(&device_token)?;
    let device = body.get("device").cloned().unwrap_or(Value::Null);
    let summary = authenticate_session(base_url, device_token.clone(), &state).await?;
    Ok(PairDeviceResult {
        summary,
        device_token,
        device,
    })
}

#[tauri::command]
fn disconnect_server(state: tauri::State<'_, AppState>) -> Result<(), String> {
    *state
        .session
        .lock()
        .map_err(|_| "client session lock is poisoned".to_string())? = None;
    Ok(())
}

#[tauri::command]
async fn server_status(state: tauri::State<'_, AppState>) -> Result<ServerSummary, String> {
    let session = current_session(&state)?;
    summary(&session).await
}

#[tauri::command]
async fn list_tasks(state: tauri::State<'_, AppState>) -> Result<Value, String> {
    let session = current_session(&state)?;
    api_json(&session, Method::GET, &["v1", "tasks"], None).await
}

#[tauri::command]
async fn create_task(input: String, state: tauri::State<'_, AppState>) -> Result<Value, String> {
    let input = input.trim();
    if input.is_empty() || input.len() > 256 * 1024 {
        return Err("task input must contain 1-262144 bytes".into());
    }
    let session = current_session(&state)?;
    api_json(
        &session,
        Method::POST,
        &["v1", "tasks"],
        Some(json!({
            "input": input,
            "metadata": {
                "source": "kingaibot-control-center",
                "client_os": std::env::consts::OS
            }
        })),
    )
    .await
}

#[tauri::command]
async fn cancel_task(id: String, state: tauri::State<'_, AppState>) -> Result<(), String> {
    validate_id(&id)?;
    let session = current_session(&state)?;
    api_json(
        &session,
        Method::POST,
        &["v1", "tasks", &id, "cancel"],
        Some(json!({})),
    )
    .await?;
    Ok(())
}

#[tauri::command]
async fn list_approvals(state: tauri::State<'_, AppState>) -> Result<Value, String> {
    let session = current_session(&state)?;
    api_json(&session, Method::GET, &["v1", "approvals"], None).await
}

#[tauri::command]
async fn decide_approval(
    id: String,
    status: String,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    validate_id(&id)?;
    if status != "approved" && status != "denied" {
        return Err("approval status must be approved or denied".into());
    }
    let session = current_session(&state)?;
    api_json(
        &session,
        Method::POST,
        &["v1", "approvals", &id],
        Some(json!({ "status": status })),
    )
    .await?;
    Ok(())
}

#[tauri::command]
async fn list_evolution(state: tauri::State<'_, AppState>) -> Result<Value, String> {
    let session = current_session(&state)?;
    api_json(
        &session,
        Method::GET,
        &["v1", "evolution", "proposals"],
        None,
    )
    .await
}

#[tauri::command]
async fn create_device_pairing(
    args: CreatePairingArgs,
    state: tauri::State<'_, AppState>,
) -> Result<Value, String> {
    if args.expires_in_seconds > 900 {
        return Err("pairing lifetime cannot exceed 900 seconds".into());
    }
    let session = current_session(&state)?;
    api_json(
        &session,
        Method::POST,
        &["v1", "devices", "pairings"],
        Some(json!({
            "scopes": args.scopes,
            "expires_in_seconds": args.expires_in_seconds
        })),
    )
    .await
}

#[tauri::command]
async fn list_devices(state: tauri::State<'_, AppState>) -> Result<Value, String> {
    let session = current_session(&state)?;
    api_json(&session, Method::GET, &["v1", "devices"], None).await
}

#[tauri::command]
async fn revoke_device(id: String, state: tauri::State<'_, AppState>) -> Result<(), String> {
    validate_id(&id)?;
    let session = current_session(&state)?;
    api_json(
        &session,
        Method::POST,
        &["v1", "devices", &id, "revoke"],
        Some(json!({})),
    )
    .await?;
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_os::init())
        .manage(AppState::default())
        .setup(|app| {
            let salt_path = app
                .path()
                .app_local_data_dir()
                .map_err(|e| format!("could not resolve app data directory: {e}"))?
                .join("stronghold-salt");
            app.handle()
                .plugin(tauri_plugin_stronghold::Builder::with_argon2(&salt_path).build())?;
            #[cfg(any(target_os = "android", target_os = "ios"))]
            app.handle().plugin(tauri_plugin_biometric::init())?;
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            connect_server,
            pair_device,
            disconnect_server,
            server_status,
            list_tasks,
            create_task,
            cancel_task,
            list_approvals,
            decide_approval,
            list_evolution,
            create_device_pairing,
            list_devices,
            revoke_device
        ])
        .run(tauri::generate_context!())
        .expect("error while running KINGAIBOT Control Center");
}
