# DynamicMemoryGo: 轻量可配置的动态分层记忆引擎

一款给对话应用用的分层记忆工具，解决长对话里上下文记不住的老问题，照着人类记忆的逻辑做的，有动态分层、智能遗忘、语义整合，不会越用越占存储，让你的应用能有长期稳定的记忆能力。

## 核心特性

- 4层动态记忆结构：照着人脑的记忆逻辑做的，从临时的对话缓存到永久的核心配置，分层管
- 智能的遗忘逻辑：不是直接删数据，低权重的就挪去冷存储，用到的时候还能找回来
- 只存有用的内容：不存完整的原始对话，只存语义摘要和向量，存储能省90%以上
- 自动整合记忆：把零散的互动记录自动整合成长期的概念，不会攒一堆碎片数据
- 全配置化：所有层级的参数、存储的配置都能自己改，适配不同的业务
- 混合架构：Go管存储调度，性能高，Python管元数据计算，灵活好扩展
- 留了扩展口：后续要改权重计算、记忆整合的逻辑都很方便，不用动核心

## 记忆层级说明

| 层级 | 名称 | 存在哪 | 处理逻辑 |
|------|------|----------|----------|
| L0 | 工作记忆 | Redis | 临时的对话缓存，最多存20条，7天过期，没用的数据直接过滤 |
| L1 | 情景记忆 | MySQL | 重要的互动记录，存30天，没用的自动挪去冷存储，重要的会自动整合 |
| L2 | 语义记忆 | MySQL | 长期的概念，存90天，没用的自动挪去冷存储 |
| L3 | 核心记忆 | 本地文件 | 永远不会丢的核心配置，保证系统核心不会变 |

## 快速开始

### 环境准备
- Go 1.21+
- Python 3.8+
- Redis 6.0+
- MySQL 8.0+

### 拉代码装依赖
```bash
# 先把代码拉下来
git clone https://github.com/yourname/dynamic_memory_go.git
cd dynamic_memory_go

# 装Go的依赖
go mod download

# 装Python的依赖
cd python
pip install -r requirements.txt
```

### 改配置
改 `config/config.yaml` 里的存储地址这些，改成你自己的环境：
```yaml
storage:
  redis:
    addr: "127.0.0.1:6379" # 你的Redis地址
  mysql:
    dsn: "user:password@tcp(127.0.0.1:3306)/dynamic_memory?charset=utf8mb4&parseTime=True&loc=Local" # 你的MySQL地址
```

### 起服务
```bash
# 先起Python的元数据服务
cd python
python memory_service.py

# 再起Go的记忆引擎
go run main.go
```

## API 用法

### 加一条新记忆
```go
ctx := context.Background()
err := memory.AddMemory(ctx, "用户的输入文本")
```

### 查相关的记忆
```go
ctx := context.Background()
mems, err := memory.RetrieveMemory(ctx, "当前用户输入")
```

### 定时整理记忆
```go
// 每天凌晨2点跑一次就行
ticker := time.NewTicker(24 * time.Hour)
for range ticker.C {
    memory.SleepMemory()
}
```

## 自定义扩展

你可以继承 `BaseMetaCalculator` 这个类，自己写权重计算、摘要提取这些逻辑，适配你的业务：
```python
class MyMetaCalculator(BaseMetaCalculator):
    def calculate_weight(self, raw_input: str) -> float:
        # 这里写你自己的权重计算逻辑
        return my_weight
```

## 授权说明

这个项目用的双授权：
- 个人非商用：完全免费，随便用随便改
- 企业商用：需要拿个授权，有单项目的和企业全场景的两种

要拿商业授权可以找我：
- 爱发电：[购买链接]
- 邮箱：your@email.com

## 贡献

有问题或者改了什么好东西，欢迎提Issue和PR！

## License

GPL-3.0 License
