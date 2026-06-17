# web/src/router 模块说明（粤语版）

## 呢个模块做咩

`router` 系前端页面嘅「导航牌」。用户输入网址或者点击菜单，router 决定应该显示登录页、Dashboard、404 定其他页面。

## 调用关系

```text
main.ts
  -> 创建 Vue App
  -> 挂载 router
  -> 用户访问 URL
  -> router 匹配对应 view component
```

## 入参

| 入参 | 说明 |
| --- | --- |
| URL path | 例如 `/login`、`/dashboard` |
| 路由配置 | path 对应边个组件 |
| 登录状态 | 路由守卫：非 `meta.public` 路由无 token 时跳转 `/login` |

## 出参

| 输出 | 说明 |
| --- | --- |
| 页面组件 | 匹配后渲染对应 Vue 页面 |
| 路由跳转 | 登录成功、401 登出、访问错误页等 |
| `meta.activeMenu` | 可选；嵌套子路由时指定侧栏高亮项（见 `BasicLayout.vue`） |

## 通俗比喻

`router` 就似商场指示牌：你想去戏院、超市、停车场，指示牌会话你应该行边条路。