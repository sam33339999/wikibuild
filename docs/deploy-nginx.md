# 部署：Nginx / Nginx Proxy Manager + WikiBuild

架構（本專案實務）：

```text
瀏覽器  ──HTTPS──►  Nginx Proxy Manager（憑證掛這裡）
                         │
                         │ HTTP proxy_pass
                         ▼
                   Go wikibuild（本機 127.0.0.1:PORT）
                         │
                         ▼
                      PostgreSQL
```

- **TLS 終止在 NPM**，Go 只聽 HTTP。
- 登入 CSRF、session cookie、LLM SSE 都要靠 proxy **正確轉傳 Cookie / 長連線**。

相關程式：`internal/server/server.go`（CSRF、TrustProxy）、`cmd/resetadmin`（重設密碼）。

---

## 事故紀錄：上線後無法登入 / CSRF Forbidden（2026-07）

本節記錄**實際踩過的問題**與**已做的調整**（含 NPM），之後換機或重裝可對照。

### 現象

| 現象 | 說明 |
|------|------|
| 登入後只看到 **Forbidden** | 瀏覽器顯示 403，幾乎沒有說明 |
| 改了 `.env` 密碼仍登不進去 | 以為是帳密，其實常先卡 CSRF |
| `make migrate-up` 報 `migrate: not found` | 主機沒裝 migrate CLI |
| `make migrate-force` 報 `please specify version V` | 必須 `make migrate-force V=6` 這類帶版號 |

### 根因拆分（兩件不同的事）

#### A. CSRF / Forbidden（與密碼無關）

登入表單使用 **double-submit CSRF**：

| 步驟 | 必須成功 |
|------|----------|
| GET `/admin/login` | Response `Set-Cookie: csrf_=TOKEN` + 表單 hidden `_csrf=TOKEN` |
| POST `/admin/login` | Request 帶上 **同一** `Cookie: csrf_=TOKEN` 與 body `_csrf=TOKEN` |

兩邊不一致 → **403 Forbidden**（舊版幾乎無訊息）。

在 **NPM 終止 TLS → HTTP 轉 Go** 時，最常見：

1. **網域不一致**：用 `https://網域` 開頁、卻用 `http://IP:埠` 送出（或相反）→ cookie 綁錯 host。  
2. **Cookie 沒進瀏覽器 / 沒送回**：NPM 設定弄掉 `Set-Cookie`，或 cache 了舊登入頁。  
3. **Forward 目標錯誤**：沒指到本機 Go 的 port，或 Docker 網路指錯宿主。  
4. （較少）Go 端 cookie 屬性與 proxy 環境不合。

**密碼錯誤不會是 Forbidden**，而是帳密失敗（見下）。

#### B. 帳密（CSRF 通過之後才會驗）

| 重點 | 說明 |
|------|------|
| `WIKIBUILD_ADMIN_PASS` | **只在 DB 尚無該 user 時** 由 `ensureAdmin` 寫入 |
| 之後改 `.env` | **不會**自動更新 DB hash |
| 正確重設 | `go run ./cmd/resetadmin`（讀 `.env` 更新／建立 user） |

### 程式端已做的調整

| 變更 | 目的 |
|------|------|
| 登入 CSRF 失敗 → 導向 `/admin/login?err=csrf` | 不再只有空白 Forbidden |
| 帳密錯誤 → `?err=cred`；鎖帳 → `?err=locked` | 表單紅字說明 |
| CSRF `CookieSecure=false`、`Path=/`、`SameSite=Lax` | TLS 在 NPM、Go 是 HTTP 時 cookie 仍可寫入瀏覽器 |
| Fiber `TrustProxy` + 信任 loopback/private | 正確讀 `X-Forwarded-For`（限流 IP 等） |
| CSRF 錯誤文案指向本文件 | 方便運維 |
| Makefile：無 `migrate` CLI 時 `go run` 後備 | 主機不必先裝 migrate |
| `cmd/resetadmin` | 強制把 `.env` 密碼寫入 DB |

### NPM 端已對齊／建議的調整（務必）

在 **Proxy Hosts → 你的網域 → Edit**：

#### Details

| 欄位 | 設定 |
|------|------|
| Domain Names | 公開網域（例 `docs.example.com`） |
| Scheme | `http` |
| Forward Hostname / IP | **本機 Go 位址**（常是 `127.0.0.1`；NPM 在 Docker 內則用宿主 IP 或 `172.17.0.1` 等） |
| Forward Port | 與 `WIKIBUILD_PORT` 相同（例 `8880`） |
| Cache Assets | **關閉** |
| Block Common Exploits | 可開 |
| Websockets Support | 可開 |
| SSL Certificate | 憑證掛 NPM；**Force SSL** 可開 |

#### Advanced → Custom Nginx Configuration

```nginx
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Host  $host;

proxy_http_version 1.1;
proxy_set_header Connection "";

# 登入 cookie + LLM playground SSE
proxy_buffering off;
proxy_cache off;
proxy_read_timeout 3600s;
proxy_send_timeout 3600s;

# 不要：
# proxy_hide_header Set-Cookie;
# 錯誤的 proxy_cookie_path / proxy_cookie_domain;
```

#### 不要這樣做

| 錯誤 | 後果 |
|------|------|
| 對外開 `公網IP:8880` 又用 IP 登入 | 與 HTTPS 網域 cookie 分裂 |
| 只改 `.env` 密碼不跑 `resetadmin` | CSRF 過了仍 `err=cred` |
| Cache 登入／admin 頁 | 舊 `_csrf` |
| 隱藏或改寫 `Set-Cookie` | 永遠 CSRF fail |

### 建議上線檢查清單

```bash
cd ~/wikibuild
git pull
# 確認 .env：DATABASE_URL、HOST/PORT、BASE_URL=https://網域、admin 帳密、SESSION_SECRET
make migrate-up          # 或 go run … migrate（無 CLI 時 Makefile 會後備）
go run ./cmd/resetadmin  # 密碼與 .env 對齊
# 重啟 wikibuild
```

瀏覽器：

1. **無痕** + 只用 `https://網域/admin/login`  
2. 硬重新整理  
3. 登入；若失敗看頁上紅字：`csrf` / `cred` / `locked`  

F12 快速驗：

- GET login → 有 `Set-Cookie: csrf_`  
- Cookies 有 `csrf_`  
- POST login → 有帶 `Cookie` 與 `_csrf`  

---

## 1. Go 服務（本機）

```bash
# .env 建議
WIKIBUILD_HOST=127.0.0.1   # 只聽本機，由 Nginx 對外
WIKIBUILD_PORT=8880          # 與 proxy 轉發埠一致
WIKIBUILD_BASE_URL=https://你的網域   # 無尾隨斜線；公開 HTTPS
DATABASE_URL=...
WIKIBUILD_ADMIN_USER=admin
WIKIBUILD_ADMIN_PASS=...
WIKIBUILD_SESSION_SECRET=至少16字元的隨機字串

go run ./cmd/resetadmin
./wikibuild
```

本機確認：

```bash
curl -sI http://127.0.0.1:8880/admin/login | head -20
# 應 200 與 Set-Cookie: csrf_=...
```

---

## 2. Nginx Proxy Manager（圖形介面）

見上方 **「NPM 端已對齊／建議的調整」**（Details + Advanced）。

Forward 位址若 NPM 跑在 Docker：

| NPM 位置 | Forward Hostname 常見寫法 |
|----------|---------------------------|
| 與 wikibuild 同宿主、Go 聽 127.0.0.1 | 宿主 IP 或 Docker bridge gateway（**不要**只寫 127.0.0.1 除非 network_mode=host） |
| network_mode: host 的 NPM | `127.0.0.1` 即可 |

以「從 NPM 容器能否 `curl http://FORWARD:PORT/admin/login` 看到 Set-Cookie」為準。

---

## 3. 瀏覽器檢查

1. 無痕。  
2. 只用 **HTTPS 網域**（不要混 IP:port）。  
3. F12 → Network / Cookies 對照 `csrf_` 與 `_csrf`。  

| 現象 | 調哪裡 |
|------|--------|
| GET 沒有 Set-Cookie | NPM 轉發／隱藏 header |
| 有 Set-Cookie 但存不進 Cookies | 網域不一致或瀏覽器擋 cookie |
| Cookies 有、POST 沒帶 | 同 host 送出 |
| 頁面 `?err=csrf` | 同上 CSRF 鏈 |
| 頁面 `?err=cred` | `resetadmin` + 帳密 |
| 頁面 `?err=locked` | 稍後再試或重啟清 limiter（重啟 process 會清記憶體限流） |

---

## 4. curl 自測（經公開網域）

```bash
BASE='https://你的網域'
rm -f /tmp/wb.jar

curl -sS -c /tmp/wb.jar -b /tmp/wb.jar "$BASE/admin/login" -o /tmp/login.html
grep -o 'name="_csrf" value="[^"]*"' /tmp/login.html
cat /tmp/wb.jar

TOK=$(sed -n 's/.*name="_csrf" value="\([^"]*\)".*/\1/p' /tmp/login.html | head -1)

curl -sS -D- -o /dev/null -c /tmp/wb.jar -b /tmp/wb.jar \
  -X POST "$BASE/admin/login" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "username=admin" \
  --data-urlencode "password=你的密碼" \
  --data-urlencode "_csrf=$TOK"
```

| 結果 | 意義 |
|------|------|
| `303` → `/admin` | CSRF + 密碼 OK |
| `303` → `...err=csrf` | cookie／token |
| `303` → `...err=cred` | 密碼／resetadmin |
| jar 無 `csrf_` | NPM 沒轉出 Set-Cookie |

本機繞過 NPM：

```bash
curl -sS -c /tmp/local.jar -b /tmp/local.jar http://127.0.0.1:8880/admin/login | grep _csrf
```

---

## 5. Migration（主機）

```bash
# 有 migrate CLI 或 Makefile 自動 go run 後備
make migrate-up

# 僅在 schema_migrations 損壞時（需指定版號，例第 6 版）
# make migrate-force V=6
```

重設密碼**不需要** force。

---

## 6. 心智模型

```
GET  /admin/login
  ← Set-Cookie: csrf_=TOKEN   （必須進瀏覽器）
  ← HTML: <input name="_csrf" value="TOKEN">

POST /admin/login
  → Cookie: csrf_=TOKEN
  → body:  _csrf=TOKEN&username=...&password=...
  ← 303 /admin + session cookie wikibuild_admin
```

NPM 只負責 HTTPS 與轉發；**不要改寫 cookie**。憑證掛 NPM 正確；登入問題先查 **csrf_ 有沒有 Set / 有沒有送回**，再查 **resetadmin**。
