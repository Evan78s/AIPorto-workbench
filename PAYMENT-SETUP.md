# 支付宝 / 微信支付接入教程

> 前置条件：企业营业执照、已备案域名、对公账户。个人主体通常无法开通电脑网站支付/微信支付。
> 关键原则：**私钥只留在服务器 `.env`，绝不提交 git、绝不发给任何人。**

## 一、支付宝（电脑网站支付）

### 1. 申请

1. 打开 [open.alipay.com](https://open.alipay.com)，用企业账号登录。
2. 创建应用 → 添加功能「电脑网站支付」。
3. 生成应用密钥对：
   - 下载「支付宝开放平台密钥生成器」，生成 RSA2（2048 位）密钥对；
   - 得到 `应用私钥`（app_private_key.pem）和 `应用公钥`。
4. 把「应用公钥」填到开放平台应用的「应用公钥」位置，保存后平台会给出「支付宝公钥」（alipay_public_key.pem）。
   - **注意**：验签用的是「支付宝公钥」，不是应用公钥，别弄反。
5. 应用上线审核通过后拿到 `APPID`。

### 2. 配置到 Aiporto

把 PEM 文件转成单行 base64（在服务器上执行）：

```bash
base64 -w0 app_private_key.pem    # 输出贴到 ALIPAY_PRIVATE_KEY_B64
base64 -w0 alipay_public_key.pem  # 输出贴到 ALIPAY_PUBLIC_KEY_B64
```

编辑 `/www/docker/aiporto/.env`：

```ini
ALIPAY_ENABLED=true
ALIPAY_APP_ID=你的APPID
ALIPAY_PRIVATE_KEY_B64=上一步应用私钥的base64
ALIPAY_PUBLIC_KEY_B64=上一步支付宝公钥的base64
# 回调地址必须与开放平台「网关地址/回调地址」里填的一致
ALIPAY_NOTIFY_URL=https://gw.aiporto.net/api/user/pay/alipay/notify
ALIPAY_RETURN_URL=https://user.aiporto.net/orders
```

### 3. 生效与验证

```bash
cd /www/docker/aiporto
docker compose -f docker-compose.prod.yaml up -d --force-recreate gateway user-ui
```

在用户站点下单，选择支付宝，确认能跳转到支付宝收银台。付款后用测试金额验证订单状态变为「已支付」。

## 二、微信支付（v3 Native 扫码）

### 1. 申请

1. 打开 [pay.weixin.qq.com](https://pay.weixin.qq.com)，用企业账号申请商户号。
2. 拿到：`APPID`（关联的公众号/小程序 AppID）、`mch_id`（商户号）。
3. 在商户平台 → 账户中心 → API 安全：
   - 设置 APIv3 密钥（32 字节），自己保存好；
   - 申请商户 API 证书，得到 `apiclient_key.pem`（商户私钥）和证书序列号 `serial_no`。
4. 下载「微信支付平台公钥」（用于验回调签名），得到 `platform_pubkey.pem`。
   - 位置：商户平台 → API 安全 → 平台证书 → 下载公钥。
5. 配置支付回调地址：`https://gw.aiporto.net/api/user/pay/wechat/notify`。

### 2. 配置到 Aiporto

```bash
base64 -w0 apiclient_key.pem     # → WECHAT_PRIVATE_KEY_B64
base64 -w0 platform_pubkey.pem   # → WECHAT_PLATFORM_PUBLIC_KEY_B64
# APIv3 密钥本身是 32 字节字符串，需要 base64 编码后填入
echo -n "你的32字节APIv3密钥" | base64 -w0   # → WECHAT_API_V3_KEY_B64
```

编辑 `.env`：

```ini
WECHAT_ENABLED=true
WECHAT_APP_ID=你的AppID
WECHAT_MCH_ID=你的商户号
WECHAT_CERT_SERIAL_NO=商户证书序列号
WECHAT_API_V3_KEY_B64=APIv3密钥的base64
WECHAT_PRIVATE_KEY_B64=商户私钥的base64
WECHAT_PLATFORM_PUBLIC_KEY_B64=平台公钥的base64
WECHAT_NOTIFY_URL=https://gw.aiporto.net/api/user/pay/wechat/notify
```

### 3. 生效与验证

```bash
docker compose -f docker-compose.prod.yaml up -d --force-recreate gateway user-ui
```

下单选微信支付，确认订单页出现二维码。用微信扫码付款，验证订单状态更新。

## 三、常见问题

| 现象 | 原因与处理 |
| --- | --- |
| 用户端不显示支付宝/微信选项 | `ALIPAY_ENABLED` / `WECHAT_ENABLED` 未设为 true，或密钥没配全；改完未 force-recreate |
| 网关启动报 `... is required` | 对应渠道的必填项没填；看报错里的环境变量名 |
| 支付成功但订单一直待支付 | 回调地址没公网可达、域名与商户平台登记不一致、验签失败；看网关日志里的 `verify failed` |
| 回调验签失败 | 支付宝：用错公钥（要用「支付宝公钥」）；微信：平台公钥过期需重新下载 |
| 微信 `amount mismatch` | 微信回调金额单位是分，代码已自动换算；确认订单金额与实付一致 |
| 私钥填进 .env 后能不能 git commit | **不能**。.env 已在 .gitignore；只需提交 `.env.example` 的占位符 |

## 四、资金安全红线

1. 回调只接受验签通过的请求，金额与订单不符直接拒绝。
2. 订单号唯一 + 支付流水唯一约束，重复回调幂等。
3. 对公转账必须运营方在后台手动确认，系统不会自动开通。
4. 退款目前只记录流水（`refunds` 表），不自动原路退回，需人工在商户平台操作。
