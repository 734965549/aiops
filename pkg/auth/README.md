# pkg/auth 模块说明（粤语版）

## 呢个模块做咩

`auth` 系平台嘅「钥匙同门锁」。密码入库之前要加密，用户登录成功之后要发 token，请求入嚟时要验 token。

## 调用关系

```text
identity.AuthService
  -> HashPassword / VerifyPassword
  -> JWTManager.IssueAccess / IssueRefresh
  -> JWTManager.Verify

server Auth 中间件
  -> JWTAuthenticator.Authenticate
  -> JWTManager.Verify(access token)
```

## 入参

| 方法 | 入参 | 说明 |
| --- | --- | --- |
| `HashPassword` | 明文密码 | 创建默认管理员或注册时使用 |
| `VerifyPassword` | 密码 hash、明文密码 | 登录时验证 |
| `NewJWTManager` | secret、issuer、accessTTL、refreshTTL | 创建签发同验证 JWT 嘅工具 |
| `IssueAccess/IssueRefresh` | 用户 ID、用户名 | 签发 token |
| `Verify` | token、token 类型 | 校验 token 是否有效同类型是否正确 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 密码 hash | 存入数据库，唔存明文 |
| access token | 调业务接口用 |
| refresh token | 换新 token 用 |
| claims | token 入面解析出用户 ID、用户名、类型、过期时间 |
| error | 密码唔匹配、token 过期、secret 配置错等 |

## 通俗比喻

密码 hash 就似将门匙倒模成保险箱编号，别人睇到编号都唔知原匙点样。JWT 就似临时通行证，写住你系边个、几时过期、可唔可以入门。