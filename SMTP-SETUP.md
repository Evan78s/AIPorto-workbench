# SMTP 邮件配置教程

用户端注册验证码通过 SMTP 发送。不配置 SMTP 时注册接口返回 503，用户无法自助注册。

## 一、选邮件服务商

| 服务商 | SMTP 主机 | 端口 | TLS | 说明 |
| --- | --- | --- | --- | --- |
| 腾讯企业邮 | smtp.exmail.qq.com | 465 | ssl | 企业邮箱常用 |
| 腾讯企业邮 | smtp.exmail.qq.com | 587 | starttls | 也可 |
| QQ 邮箱 | smtp.qq.com | 465 | ssl | 需要用「授权码」不是登录密码 |
| 网易邮箱 | smtp.163.com | 465 | ssl | 需要开启 SMTP 并拿授权码 |
| 阿里云邮件推送 | smtpdm.aliyun.com | 80/465 | none/ssl | 需备案域名与发信域名验证 |

推荐用企业邮箱或阿里云邮件推送，到达率更稳定。

## 二、配置 .env

编辑 `/www/docker/aiporto/.env`：

```ini
SMTP_HOST=smtp.exmail.qq.com
SMTP_PORT=465
SMTP_USER=no-reply@aiporto.net
SMTP_PASS=你的邮箱密码或授权码
SMTP_FROM=Aiporto 智港 <no-reply@aiporto.net>
SMTP_TLS=ssl
```

`SMTP_TLS` 三种取值：
- `ssl`：465 端口，连接即加密；
- `starttls`：587 端口，先明文后升级加密；
- `none`：内网 relay 或本地调试（不加密，生产不建议）。

## 三、生效与验证

```bash
cd /www/docker/aiporto
docker compose -f docker-compose.prod.yaml up -d --force-recreate gateway user-ui
```

然后打开 `https://user.aiporto.net/register` 注册一个测试邮箱，检查是否收到验证码。

## 四、常见问题

| 现象 | 原因与处理 |
| --- | --- |
| 注册报 `email service is not configured` | `SMTP_HOST` 留空，或改完未 force-recreate |
| 收不到邮件 | 查网关日志里 `smtp send failed after retry`；核对账号密码/授权码 |
| 535 认证失败 | 用的是登录密码而非授权码；QQ/网易需在邮箱设置里开启 SMTP 并生成授权码 |
| 连接超时 | 服务器出站 25/465/587 端口被云厂商封禁（阿里云默认封 25）；改用 465 或申请解封 |
| 邮件进垃圾箱 | 配置发信域名 SPF / DKIM 记录；用企业邮或邮件推送服务 |
| 主题乱码 | 代码已对中文主题做 UTF-8 Base64 编码，无需处理 |
