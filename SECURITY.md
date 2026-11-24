# 安全红线与实现说明

四条红线都是代码级约束，不是"注意一下"。下面逐条说明实现位置和验证方法。

---

## 1. 上游 key 明文永不出现在日志、接口响应、前端代码

**存储**：`upstream_keys.encrypted_key BYTEA`，AES-256-GCM，布局为 `nonce(12) || ciphertext || tag`。每次加密使用新的随机 nonce，相同明文两次加密结果不同。

**主密钥**：只从环境变量 `UPSTREAM_MASTER_KEY_B64` 读取（32 字节 base64），在 `internal/config/config.go` 里校验长度；不落库、不写日志、不进镜像。

**加解密函数**（`internal/util/util.go`，可直接引用）：

```go
func EncryptKey(masterKey, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)      // 32 字节 → AES-256
	if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return nil, err }
	nonce := make([]byte, gcm.NonceSize())      // 12 字节，crypto/rand
	if _, err := rand.Read(nonce); err != nil { return nil, err }
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func DecryptKey(masterKey, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return nil, err }
	if len(blob) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)         // tag 不匹配会报错
}
```

**运行时**：`internal/routing/routing.go` 解密后只在内存里构造 `Authorization` 头，`internal/proxy/proxy.go` 每次尝试结束都调用 `clear(c.PlainKey)` 主动擦除。接口只返回 `key_hint`（如 `sk-…9f8e`），不返回 `encrypted_key` 或明文。前端源码里没有上游 key 相关字段。

**验证**：`./scripts/check-redaction.sh`；`go test ./internal/util -run 'Encrypt|Decrypt'`（随机 nonce、篡改拒绝、错误密钥拒绝）。

---

## 2. usage_logs 默认只存 token 数与元数据，不存 prompt 原文

表字段（`migrations/001_mvp.sql`）只有 request_id、tenant_id、key_id、provider_id、model、endpoint、stream、prompt_tokens、completion_tokens、status_code、latency_ms、ttft_ms、cost、created_at。没有也不会有正文列。

`internal/metering/metering.go` 的 `CopyFrom` 使用固定列白名单，即使将来 `Record` 结构体加了字段也不会自动写进去。

响应正文只在代理进程内存里短暂经过 `capture`（上限 256KB），用于提取上游 `usage` 或本地估算 token，随后即丢弃；这段缓冲不写日志、不落库。`internal/consoleapi` 的 CSV 导出同样走白名单结构体，单元测试 `TestCSVExportAllowlist` 校验表头与禁用字段。

**验证**：`./scripts/check-redaction.sh`；`go test ./internal/consoleapi -run 'CSV|Audit'`。

---

## 3. 所有写操作进 audit_logs

`internal/consoleapi/audit.go` 的 `auditWrite` 中间件挂在 `/api/admin` 与 `/api/console` 两个路由组的**最外层**，覆盖除 GET/HEAD/OPTIONS 之外的所有方法，连鉴权失败的写尝试也会记录。

记录内容：request_id、actor（管理员用户名或 `console:<租户>`）、IP、User-Agent、method+path、tenant_id、状态码、耗时。审计写入用 `context.Background()`，客户端中途断连也照样落库。

关键约束：`AuditDetail()` 只序列化路径、query 字段名、状态码、耗时。**不序列化请求体、Authorization 头或任何 key 字段**——创建租户/轮换 key 的响应里出现过的一次性明文，不会二次进入审计表。单元测试 `TestAuditDetailNeverContainsBodyOrAuth` 直接构造带明文 key 的请求做断言。

抽查审计：

```sql
SELECT created_at, actor, action, status_code, detail
FROM audit_logs ORDER BY created_at DESC LIMIT 20;
```

---

## 4. 管理后台必须双因素登录

已移除原来的 HTTP Basic Auth。现在登录分两步：

1. `POST /api/admin/auth/login`：校验 `ADMIN_USER` / `ADMIN_PASSWORD`，只返回 90 秒有效的 challenge，**不签发可访问后台的会话**。
2. `POST /api/admin/auth/verify`：校验 TOTP 6 位动态码（RFC6238，HMAC-SHA1，30 秒窗口，允许 ±1 窗口），成功后写 `admin_totp_used` 防重放，再签发 8 小时 HMAC 会话令牌。

所有业务接口要求 `Authorization: Bearer <会话令牌>`；没有第二步就返回 401。前端只在 `sessionStorage` 保存会话令牌，关闭标签页即失效，不持久化账号密码。

TOTP 私钥由你自己生成并保管（代码里没有默认值）：

```bash
openssl rand -base64 20 | tr -d '\n' | base32 | tr -d '='
```

把输出填入 `.env` 的 `ADMIN_TOTP_SECRET`，同时在开源验证器 App（如 Aegis / FreeOTP）里录入同一个私钥，用生成的 6 位码完成第二步。丢失私钥需要重新生成并替换 `.env`。

**验证**：未登录访问 `/api/admin/tenants` 应 401；错误动态码应 401；同一动态码第二次提交应 409；成功后 `audit_logs` 里能看到登录与后续写操作。

---

## 5. 一键自检

```bash
mkdir -p .toolcache && ./scripts/check-redaction.sh
```

六项检查全 PASS 才允许上线；任何 FAIL 都必须先修复。
