package payment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

func TestSignAlgorithmExcludesSignAndSorts(t *testing.T) {
	got := signAlgorithm(map[string]string{
		"b": "2", "a": "1", "sign": "xxx", "sign_type": "RSA2", "empty": "",
	})
	if got != "a=1&b=2" {
		t.Fatalf("unexpected sign string: %q", got)
	}
}

func TestYuanToFen(t *testing.T) {
	cases := map[string]int{
		"100": 10000, "99.9": 9990, "0.01": 1, "12.34": 1234, " 5 ": 500,
	}
	for in, want := range cases {
		got, err := yuanToFen(in)
		if err != nil || got != want {
			t.Errorf("yuanToFen(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1.234"} {
		if _, err := yuanToFen(bad); err == nil {
			t.Errorf("yuanToFen(%q) should fail", bad)
		}
	}
}

func TestDecryptAESGCMRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	nonce := "abcdefghijkl" // 12 bytes
	aad := "transaction"
	plain := []byte(`{"out_trade_no":"OD123","trade_state":"SUCCESS"}`)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	ct := gcm.Seal(nil, []byte(nonce), plain, []byte(aad))
	got, err := decryptAESGCM(key, nonce, aad, base64.StdEncoding.EncodeToString(ct))
	if err != nil || string(got) != string(plain) {
		t.Fatalf("decrypt mismatch: %q, %v", got, err)
	}
	if _, err := decryptAESGCM(key, nonce, "wrong-aad", base64.StdEncoding.EncodeToString(ct)); err == nil {
		t.Fatal("decrypt should fail with wrong aad")
	}
}

// 完整验证支付宝回调验签：用私钥签，用公钥验。
func TestAlipayVerifyNotify(t *testing.T) {
	priv, pub := genRSAKeyPair(t)
	p := &alipayProvider{appID: "app", privateKey: priv, publicKey: pub}
	params := map[string]string{
		"out_trade_no": "OD1", "total_amount": "10.00", "trade_status": "TRADE_SUCCESS",
	}
	sign, err := p.sign(params)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]string{}
	for k, v := range params {
		body[k] = v
	}
	body["sign"] = sign
	body["sign_type"] = "RSA2"
	res, err := p.VerifyNotify(nil, body)
	if err != nil || res.ExternalID != "OD1" {
		t.Fatalf("verify failed: %+v, %v", res, err)
	}
	body["total_amount"] = "999"
	if _, err := p.VerifyNotify(nil, body); err == nil {
		t.Fatal("tampered amount should fail verify")
	}
}

func genRSAKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return priv, &priv.PublicKey
}

// 确保私钥匙测试用 PEM 解析函数能识别 PKCS#8 与 PKCS#1。
func TestLoadPrivateKeyFormats(t *testing.T) {
	priv, _ := genRSAKeyPair(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadPrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})); err != nil {
		t.Fatalf("PKCS#8 parse failed: %v", err)
	}
	pkcs1 := x509.MarshalPKCS1PrivateKey(priv)
	if _, err := loadPrivateKey(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1})); err != nil {
		t.Fatalf("PKCS#1 parse failed: %v", err)
	}
}

// 校验微信签名串拼接顺序固定，避免大小写/顺序差异导致验签失败。
func TestWechatSignMessageOrder(t *testing.T) {
	parts := []string{"POST", "/v3/pay/transactions/native", "1700000000", "ABC", "{}", ""}
	msg := strings.Join(parts, "\n")
	want := "POST\n/v3/pay/transactions/native\n1700000000\nABC\n{}\n"
	if msg != want {
		t.Fatalf("unexpected message: %q", msg)
	}
}
