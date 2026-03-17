# 测试规范
- 单元测试覆盖 Reconcile 各分支
- 使用 envtest 模拟 API Server
- 集成测试验证端到端流程
- 测试执行命令 `go test -v`，仅执行改动的测试文件

## 单元测试模板

```go
func TestFunctionName(t *testing.T) {
    type args struct {
        param string
    }

    tests := []struct {
        name    string
        args    args
        want    string
        wantErr bool
    }{
        {
            name:    "success case",
            args:    args{param: "value"},
            want:    "expected",
            wantErr: false,
        },
        {
            name:    "error case",
            args:    args{param: ""},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Function(tt.args.param)
            if (err != nil) != tt.wantErr {
                t.Errorf("Function() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Function() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## 集成测试模板

### 使用 "github.com/onsi/ginkgo" 和 "github.com/onsi/gomega" 框架

```go
It("namespace created", func() {
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: namespaceUT,
        },
    }

    Expect(c.Create(context.TODO(), ns)).Should(Succeed())
})
```

## 测试文件位置

- 单元测试文件与源文件同目录
- 命名：`xxx_test.go`
