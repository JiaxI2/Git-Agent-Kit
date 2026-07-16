# 配置说明

配置位于目标仓库 `.gia/config.json`。

## validation.profiles

每个 profile 是按顺序执行的 shell 命令数组。命令失败立即停止。

C/C++ 示例：

```json
{
  "smoke": ["cmake -S . -B build", "cmake --build build"],
  "full": ["cmake -S . -B build", "cmake --build build", "ctest --test-dir build --output-on-failure"],
  "release": ["cmake -S . -B build-release -DCMAKE_BUILD_TYPE=Release", "cmake --build build-release", "ctest --test-dir build-release --output-on-failure"]
}
```

Node.js 示例：

```json
{
  "smoke": ["npm ci", "npm run lint"],
  "full": ["npm ci", "npm test", "npm run build"],
  "release": ["npm ci", "npm test", "npm run build", "npm pack --dry-run"]
}
```

## notifications

```json
{
  "console": true,
  "webhookUrl": "",
  "command": ["powershell", "-File", "scripts/notify.ps1", "{event}", "{message}"]
}
```

Webhook 接收 JSON：`event`、`message`、`time`。

## 安全默认值

- `requireExpectedHead=true`：验证必须指定预期 SHA；
- `requireClean=true`：验证前后工作区必须干净；
- `refuseDirtyRemoval=true`：拒绝删除脏 worktree；
- `rejectForcePush=true`：策略声明禁止 force push。
