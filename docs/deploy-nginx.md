# 部署：Nginx / Nginx Proxy Manager + WikiBuild

架構：**瀏覽器 → HTTPS（Nginx 憑證）→ HTTP → Go（wikibuild）**

登入出現 **「CSRF 驗證失敗」** 時，幾乎一定是 **CSRF cookie（`csrf_`）沒有被瀏覽器存下或沒有在 POST 時送回**，與帳密無關。

---

## 1. Go 服務（本機）

```bash
# .env 建議
WIKIBUILD_HOST=127.0.0.1   # 只聽本機，由 Nginx 對外
WIKIBUILD_PORT=8880          # 與 proxy 轉發埠一致
WIKIBUILD_BASE_URL=https://你的網域   # 無尾隨斜線；給 feed/canonical 用
DATABASE_URL=...
WIKIBUILD_ADMIN_USER=admin
WIKIBUILD_ADMIN_PASS=...
WIKIBUILD_SESSION_SECRET=至少16字元的隨機字串

# 重設 admin 密碼寫入 DB
go run ./cmd/resetadmin

# 啟動
./wikibuild   # 或 systemd / go run
```

確認本機可開（在 server 上）：

```bash
curl -sI http://127.0.0.1:8880/admin/login | head -20
# 應看到 200 與 Set-Cookie: csrf_=...
```

---

## 2. Nginx Proxy Manager（圖形介面）

### Proxy Host

| 欄位 | 建議值 |
|------|--------|
| Domain Names | 你的公開網域（例 `docs.example.com`） |
| Scheme | `http` |
| Forward Hostname / IP | `127.0.0.1` 或 `host.docker.internal`（NPM 在 Docker 內時） |
| Forward Port | `8880`（與 `WIKIBUILD_PORT` 相同） |
| Cache Assets | **關閉**（至少不要 cache `/admin`） |
| Block Common Exploits | 可開 |
| Websockets Support | 可開（非必須） |
| SSL | 你的憑證；Force SSL 可開 |

### Custom locations / Advanced

在該 Proxy Host 的 **Advanced → Custom Nginx Configuration** 貼入（或確認等價設定）：

```nginx
# 必須把真實 Host / 協定傳給 Go（cookie、導向、之後 Secure 才正確）
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Host  $host;

proxy_http_version 1.1;
proxy_set_header Connection "";

# 不要緩衝 SSE（LLM playground stream）
proxy_buffering off;
proxy_cache off;
proxy_read_timeout 3600s;
proxy_send_timeout 3600s;

# Cookie：不要改寫 / 剝掉
# （預設 proxy_pass 會轉傳 Cookie / Set-Cookie；勿加會弄壞 cookie 的 rewrite）
```

### 常見錯誤設定（請避免）

| 設定 | 問題 |
|------|------|
| Forward 到 `http://公網IP:8880` 又開錯防火牆 | 繞過 NPM，cookie 網域不一致 |
| 用 `https://網域` 開登入、卻 bookmark 成 `http://IP:8880` | cookie 綁在不同 host |
| 對 `/admin` 開 **Cache** | 舊的 `_csrf` 表單 token |
| 自訂 `proxy_hide_header Set-Cookie` | 瀏覽器永遠沒有 `csrf_` |
| 自訂 `proxy_cookie_path` / `proxy_cookie_domain` 寫錯 | cookie 路徑／網域錯誤 |

---

## 3. 瀏覽器檢查（最快定位）

1. 開 **無痕視窗**。
2. 只使用 **HTTPS 網域** 開 `/admin/login`（不要混 IP）。
3. F12 → **Network**：
   - **GET** `/admin/login` → Response Headers 應有  
     `Set-Cookie: csrf_=...; Path=/; ...`
   - Application → Cookies → 你的網域 → 應有 **`csrf_`**
   - **POST** `/admin/login` → Request Headers 應有  
     `Cookie: csrf_=...`  
     Form Data 應有 `_csrf=...`（與 cookie 值相同）
4. 若 GET 沒有 `Set-Cookie`，問題在 **Nginx／NPM 沒把上游 cookie 傳出**。  
   若有 Set-Cookie 但 POST 沒有 Cookie，問題在 **瀏覽器擋 cookie** 或 **網域不一致**。

---

## 4. 主機 curl 自測（模擬瀏覽器 cookie 罐）

把 `https://你的網域` 換成實際網域：

```bash
BASE='https://你的網域'
rm -f /tmp/wb.jar

# 1) 拿登入頁 + cookie
curl -sS -c /tmp/wb.jar -b /tmp/wb.jar "$BASE/admin/login" -o /tmp/login.html
grep -o 'name="_csrf" value="[^"]*"' /tmp/login.html
cat /tmp/wb.jar   # 應看到 csrf_

# 2) 抽出 token
TOK=$(sed -n 's/.*name="_csrf" value="\([^"]*\)".*/\1/p' /tmp/login.html | head -1)
echo "token=$TOK"

# 3) 登入（應 303 Location: /admin）
curl -sS -D- -o /dev/null -c /tmp/wb.jar -b /tmp/wb.jar \
  -X POST "$BASE/admin/login" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "username=admin" \
  --data-urlencode "password=你的密碼" \
  --data-urlencode "_csrf=$TOK"
```

| 結果 | 意義 |
|------|------|
| `303` 且 `Location: /admin` | CSRF + 密碼都 OK |
| `303` 且 `Location: ...err=csrf` | cookie／token 仍沒對上（proxy 或 cookie） |
| `303` 且 `err=cred` | CSRF 過了，**密碼／resetadmin** 問題 |
| jar 裡沒有 `csrf_` | **Nginx 沒轉出 Set-Cookie** |

本機直連 Go（繞過 NPM）對照：

```bash
curl -sS -c /tmp/local.jar -b /tmp/local.jar http://127.0.0.1:8880/admin/login | grep _csrf
# 若本機有 csrf、走網域沒有 → 只改 Nginx/NPM
```

---

## 5. 帳密（與 CSRF 分開）

CSRF 過了才會驗密碼。重設：

```bash
cd ~/wikibuild
# 編輯 .env 的 WIKIBUILD_ADMIN_USER / WIKIBUILD_ADMIN_PASS
go run ./cmd/resetadmin
# 重啟 wikibuild
```

---

## 6. 心智模型

```
GET  /admin/login
  ← Set-Cookie: csrf_=TOKEN   （必須進瀏覽器）
  ← HTML: <input name="_csrf" value="TOKEN">

POST /admin/login
  → Cookie: csrf_=TOKEN       （必須帶上）
  → body:  _csrf=TOKEN&username=...&password=...
  ← 303 /admin + session cookie
```

兩邊 TOKEN 一致才通過。Nginx 只轉 HTTP，**不要改 cookie**。
