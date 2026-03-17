# 编码规范

## 命名约定

| 类型 | 格式 | 示例 |
|------|------|------|
| 文件 | 蛇形命名 | `agentic_version_create.go` |
| 结构体 | PascalCase | `AgenticVersionCreateRequest` |
| 方法 | CamelCase，动词开头 | `CreateVersion` |
| 常量 | 驼峰或全大写下划线 | `DefaultEnvPre` |

## 包导入顺序

```go
import (
    // 标准库
    "context"
    "fmt"
    "time"

    // 第三方库
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
    "go.mongodb.org/mongo-driver/bson"

    // 项目内部包
    "code.alipay.com/paas-core/kox/pkg/model"
    "code.alipay.com/paas-core/kox/pkg/logging"
)
```
