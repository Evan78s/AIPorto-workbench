# Aiporto 用户端（注册 / 登录 / 购买）

> 品牌：Aiporto｜智港 · 万模入港，统一调度
>
> 本文档面向**运营方**：如何部署用户站点、配置支付与邮件、以及用户完整购买流程。

## 一、这套系统解决了什么

原系统只有运营后台和控制台，客户要靠运营方手动签发 key。用户端补上了自助闭环：

```
用户注册 → 邮箱验证 → 登录 → 选套餐 → 支付 → 自动开通 API Key + 控制台令牌 → 用户调用网关
```

- 用户站点独立域名：`user.aiporto.net`（Docker 容器 `user-ui`，宿主机 8082 端口）
- 网关域名：`gw.aiporto.net`（继续用原来的反代，API 与支付回调都在这里）
- 运营后台：`admin.aiporto.net`（不变）

## 二、升级部署（已有服务器）

### 1. 拉取新代码并重建

```bash
cd /www/docker/aiporto
git pull
# 用户端容器首次需要构建
docker compose -f docker-compose.prod.yaml up -d --build --force-recreate gateway user-ui admin-ui
```

> 注意：数据库迁移 `004_user_portal.sql` **不会**自动在已有库执行（Compose 的 initdb 只在首次初始化空库时跑）。必须按下一步手动补跑。

### 2. 补跑数据库迁移

```bash
cd /www/docker/aiporto
bash scripts/migrate-user-portal.sh
```

看到 `users / email_tokens / user_sessions / orders / payments / refunds` 六张表即成功。该脚本可重复执行。

### 3. 配置 .env

在 `.env` 至少补充（完整说明见 `.env.example`）：

```ini
USER_PORTAL_URL=https://user.aiporto.net
PUBLIC_BASE_URL=https://gw.aiporto.net

# 邮件必填，否则注册接口返回 503
SMTP_HOST=smtp.your-provider.com
SMTP_PORT=587
SMTP_USER=your-account
SMTP_PASS=your-password
SMTP_FROM=Aiporto 智港 <no-reply@aiporto.net>
SMTP_TLS=starttls

# 支付按需启用，均未启用则只显示对公转账
ALIPAY_ENABLED=false
WECHAT_ENABLED=false
```

改完 `.env` **必须**重建容器才会生效：

```bash
docker compose -f docker-compose.prod.yaml up -d --force-recreate gateway user-ui
```

> `docker compose restart` 不会重新读取 `.env`，务必用 `up -d --force-recreate`。

### 4. 宝塔新增用户站点

1. 宝塔 → 网站 → 添加站点，域名填 `user.aiporto.net`，目录随意（纯反代）。
2. 站点设置 → 反向代理 → 添加反代：
   - 目标 URL：`http://127.0.0.1:8082`
   - 发送域名：`$host`
3. 配置文件里加上 `proxy_buffering off;`（与 admin 站点一致）。
4. 申请 Let's Encrypt SSL，强制 HTTPS。

**网关站点（`gw.aiporto.net`）还需新增支付回调的反代**——它走的是 `/api/user/pay/...`，已被现有 `/api/` 反代覆盖，无需额外配置。但要确保：
- 回调地址是公网可访问的 HTTPS（`PUBLIC_BASE_URL` 填 `https://gw.aiporto.net`）；
- 支付宝/微信商户平台里登记的回调地址与 `.env` 中的 `ALIPAY_NOTIFY_URL` / `WECHAT_NOTIFY_URL` 完全一致。

## 三、用户完整流程（自助）

1. 打开 `https://user.aiporto.net`，点「免费注册」，填邮箱和密码。
2. 收到验证码邮件，回填 6 位验证码 + 再次输入密码，注册成功。
3. 浏览器保持登录态（令牌只存 sessionStorage，关闭标签页需重新登录）。
4. 进入「套餐」选择，选支付宝 / 微信 / 对公转账，确认支付。
5. 支付完成后：
   - 支付宝：跳转回用户站点的订单页；
   - 微信：订单页显示二维码，扫码后点「刷新状态」；
   - 对公转账：页面显示收款信息，线下打款后等待运营方在后台确认。
6. 订单状态变为「已支付」后，点「领取密钥」，一次性拿到：
   - `sk-wb-...`（调用网关的 API Key）
   - `cw-...`（客户控制台令牌）
   
   **明文只显示这一次**，请立即保存。之后可在客户控制台用 `cw-` 令牌登录查看用量、管理 key。

## 四、运营方后台操作

运营后台（`/admin`）新增菜单数据接口（当前需通过 API 调用，后续可加前端页面）：

```bash
# 订单列表
curl -H "Authorization: Bearer <管理员会话令牌>" https://admin.aiporto.net/api/admin/user-orders

# 确认对公转账（把订单号换成用户的订单号）
curl -X POST -H "Authorization: Bearer <管理员会话令牌>" \
  https://admin.aiporto.net/api/admin/user-orders/AP20260101123456-ABC12345/confirm
```

确认后系统会自动开通套餐、API Key、控制台令牌，用户订单页状态自动更新。

## 五、安全说明

- 密码用 bcrypt 存储；会话令牌、API Key、控制台令牌都只存 HMAC 摘要，明文只返回一次。
- 邮箱验证码只存 HMAC 摘要，5 分钟有效，最多 5 次尝试。
- 支付回调验签 + 金额比对 + 订单号幂等，重复回调不会重复开通；原始回调留痕在 `payments.raw_notify`。
- 领取密钥的接口用行锁，防止并发重复领取；领取后明文临时字段立即清空。
- 私钥（支付宝/微信）在 `.env` 里只以 base64(PEM) 形式存在，绝不写进代码或前端。
