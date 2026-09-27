# 第 11 章 并发进阶：Context、调度器与并发模式

> 本章导读：第 10 章我们已经会用 goroutine 与 channel 写出并发程序，但"能跑通"和"能在生产环境里跑"之间还隔着三道坎：取消与超时如何跨层传递、运行时究竟怎样调度成千上万个 goroutine、以及并发逻辑如何组织才不会变成一团乱麻。本章先逐个解剖 `context` 包的 API 与使用规范，再下潜到 GMP 调度器和 Go 内存模型，最后给出十三个可直接抄用的并发模式、一套错误处理范式、一份调优手册和一张贴在工位上的检查清单。

## 11.1 Context 包全景

### 11.1.1 为什么需要 Context：没人喊停的 goroutine

先看一个真实困境。你写了一个 HTTP 接口，它调用业务层，业务层调用数据库，数据库再调用下游 RPC。现在客户端在请求发出 200 毫秒后断开了连接，或者上游的负载均衡器因为超时把连接掐了。请问：

1. 谁来通知这场调用链上的所有环节"别再干了"？
2. 数据库驱动正在执行的查询怎么被中断？它可不知道 HTTP 客户端已经走了。
3. 这些 goroutine 什么时候退出？如果不退出，它们占的内存、连接、锁又归谁回收？

第 10 章的知识能解决其中一部分：我们可以给每个调用加一个 `done chan struct{}` 参数，谁想取消就把它关掉（或往里塞一个值）。但这条路很快会崩掉：

- **参数污染**。每个函数都要多一个 `done` 参数，跨 5 层调用就要传 5 次，每个中间层都得记住"收到就往下传"。
- **信息量不足**。是超时了，还是上游主动取消了？调用方只能猜。
- **表达能力不够**。如果还想顺带传一个 traceID 做链路追踪，再加一个参数？那再加个 userID 呢？
- **不可组合**。父任务取消要级联到所有子任务，子任务超时又只影响自己这一支，靠手工传 channel 几乎无法维护。

`context` 包（Go 1.7 进入标准库）就是为此而生的：它把"取消信号 + 截止时间 + 请求域元数据"三件事打包成一个不可变的、可派生的、并发安全的对象，沿着调用链一路向下传。它最重要的设计哲学是——**取消是协作式的**：Context 不会杀死任何 goroutine，它只负责"广播一个信号"，具体每个 goroutine 是否响应、何时响应，由你自己在代码里 `select` 出来。这意味着写并发代码时你必须回答一个问题："我这个 goroutine 的退出路径在哪里？"

💡 提示：Go 官方博客的说法是"Context 应该沿着调用链传递，而不是被存储"。本章 11.1.9 会把这条规范拆成 8 条可执行的规则。

### 11.1.2 Context 接口：四个方法逐个精讲

`context.Context` 只有四个方法，但每一个都有容易踩的细节（下面是标准库中的接口定义，片段）：

```go
type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}
```

**`Deadline() (time.Time, bool)`**：返回这个 Context 何时会被自动取消。第二个返回值 `ok` 为 `false` 表示"不会自动取消"（例如 `context.Background()`）。它的典型用途是给下游留足预算：如果上游只剩 50 毫秒了，我就不要再设一个 500 毫秒的超时，否则纯属自欺欺人。注意 `Deadline` 返回的是绝对时间点而不是剩余时长，比较时要和 `time.Now()` 对齐。

**`Done() <-chan struct{}`**：返回一个只读 channel，**Context 被取消时它会被关闭**。注意是"关闭"而不是"发一个值"，因此所有等待者会同时被唤醒，并且可以无限次地读——这是广播语义的标准实现。它可能返回 `nil`（比如 `Background()`），而**从 nil channel 读取会永久阻塞**，所以"一个永远不会被取消的 Context"在 `select` 里等价于禁用这一路分支。

**`Err() error`**：返回取消的原因，只有三种结果：未取消时为 `nil`，被主动取消时是 `context.Canceled`，到期时是 `context.DeadlineExceeded`。判定时必须用 `errors.Is`，原因见 11.1.5。

**`Value(key any) any`**：取出请求域数据。查找会沿派生链向上遍历，直到命中或到达根节点返回 `nil`。它是四个方法里唯一"到处都在用、也最容易用错"的一个。

### 11.1.3 创建函数与树形结构

标准库把"创建"这件事收敛成了极少数几个函数：

| 函数 | 作用 | 何时取消 |
| --- | --- | --- |
| `context.Background()` | 返回根 Context，永不取消，无值 | 从不 |
| `context.TODO()` | 占位用的根 Context，语义是"我还没想好该用哪个" | 从不 |
| `context.WithCancel(parent)` | 派生可手动取消的子 Context | 调用 `cancel()` 时 |
| `context.WithTimeout(parent, d)` | 派生带超时的子 Context | `d` 之后或 `cancel()` 时 |
| `context.WithDeadline(parent, t)` | 派生带绝对截止时间的子 Context | 到达 `t` 或 `cancel()` 时 |
| `context.WithValue(parent, k, v)` | 派生存放元数据的子 Context | 继承父的取消行为 |
| `context.WithoutCancel(parent)` | 脱敏取消、保留值（Go 1.21+） | 从不 |

每次派生都返回一个**新**的 Context，它的父节点是传入的那个，于是所有派生关系天然构成一棵树：

```text
                    context.Background()          <- 根：永不取消
                            |
          +-----------------+------------------+
          |                                    |
   WithValue(traceID)                   WithCancel(用户会话)
          |                                    |
   WithTimeout(50ms) --------+           WithValue(userID)
          |                  |                 |
   <-- ctx.Done() 关闭 -->   |          WithTimeout(3s)
   (级联关闭整棵子树)         |                 |
                      WithCancel(子查询)   WithCancel(轮询任务)
```

取消是**向下级联**的：任何一个节点被取消，它的所有子孙节点都会立刻被取消，`Done()` 依次关闭；但**不会向上影响父节点**——子任务超时不该把整个服务拖下水。反过来，父节点一旦取消，子节点无论还剩多少预算都立即失效，这也是"超时预算必须自上而下分配"的根本原因。

### 11.1.4 cancel 必须调用：一次泄漏的真实代价

这是 `context` 最著名的一条坑，也是 `go vet` 里专门有一条 `lostcancel` 检查的原因。以下是错误的两种写法与正确写法（片段，只保留关键几行）：

```go
// 错误写法：cancel 没有被调用
func query(ctx context.Context) {
	sub, _ := context.WithTimeout(ctx, time.Second) // 丢弃了 cancel
	doQuery(sub)
}

// 正确写法
func query(ctx context.Context) {
	sub, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel() // 必须调用
	doQuery(sub)
}
```

为什么即使"父 Context 迟早会被取消"也必须自己调？因为 `WithCancel`/`WithTimeout` 派生出的子 Context 会把自己注册进父节点的**孩子列表**，这样父节点取消时才能找到它并级联。`cancel()` 的职责有两个：一是把自己从父节点的孩子列表里摘掉，二是释放内部的定时器（`WithTimeout` 会启动一个定时器）。如果你不调用它，这个孩子节点会一直挂在父节点上，定时器也会一直活着，直到父节点被取消——在长生命周期的父 Context（比如服务器进程的根 Context）下，这就是教科书式的**内存泄漏**：每分钟 1000 个请求，每个泄漏几百字节，一天下来就是几百兆。

⚠️ 注意：`defer cancel()` 要尽可能早地写、紧挨着创建语句，不要被后面的 `if` 分支或 `return` 绕过去。另外 `cancel()` **是幂等的**，重复调用安全，多个 goroutine 并发调用也安全，所以不用担心"会不会重复取消导致 panic"。

写对之后的典型形态是这样的：

```go
// 摘自本章验证用例 a01_ctx_cancel/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func main() {
	// 1) WithCancel：由调用方决定何时取消
	ctx, cancel := context.WithCancel(context.Background())
	fmt.Printf("Done() 的静态类型: %T\n", ctx.Done())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel() // 取消会沿树向下传播到所有子 Context
	}()

	// 2) 用 select 等待取消信号，这是 Context 最核心的消费方式
	select {
	case <-time.After(time.Second):
		fmt.Println("等待超时，没有收到取消信号")
	case <-ctx.Done():
		fmt.Println("收到取消信号:", ctx.Err())
	}

	// 3) 判断取消原因必须用 errors.Is，不要用字符串比较
	fmt.Println("errors.Is(Canceled) =", errors.Is(ctx.Err(), context.Canceled))

	// 4) WithTimeout：到点自动取消
	tctx, tcancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer tcancel()
	dl, ok := tctx.Deadline()
	fmt.Println("存在截止时间:", ok, "尚未到期:", time.Until(dl) > 0)
	<-tctx.Done()
	fmt.Println("超时原因:", tctx.Err())
	fmt.Println("errors.Is(DeadlineExceeded) =", errors.Is(tctx.Err(), context.DeadlineExceeded))

	// 5) cancel 幂等：重复调用安全，不会 panic
	cancel()
	cancel()
	fmt.Println("重复调用 cancel 安全")
}
```

运行结果：

```text
Done() 的静态类型: <-chan struct {}
收到取消信号: context canceled
errors.Is(Canceled) = true
存在截止时间: true 尚未到期: true
超时原因: context deadline exceeded
errors.Is(DeadlineExceeded) = true
重复调用 cancel 安全
```

解读几个关键点：

- `Done()` 的类型是 `<-chan struct{}`——只读 channel，调用方不可能误往里写，也无法关闭它，只能由 Context 内部关闭。这就是"能力最小化"的接口设计。
- `select` 里同时放 `time.After` 和 `ctx.Done()` 是**错误示范**，正确做法是让 ctx 自己带超时（`WithTimeout`），再只 `select` 一个 `ctx.Done()`。因为 `time.After` 会创建定时器，在循环里用会持续堆积到超时后才被回收，造成 GC 压力。
- 同一个 `cancel()` 调用两次不会 panic，程序照常运行——这让我在 `defer cancel()` 之外还能安全地提前取消。

### 11.1.5 Err() 的语义与 errors.Is 的判定

`Err()` 只返回三种值（`nil` / `context.Canceled` / `context.DeadlineExceeded`），但**既不要用 `err.Error()` 做字符串比较，也不要对可能被包装过的错误用 `==`**（片段）：

```go
// 错误：字符串比较把"判定错误类型"变成了"比对文案"，
// 文案一改（升级标准库、加一层包装）就失效
if ctx.Err().Error() == "context deadline exceeded" { ... }

// 错误：err 往往已经被若干层 fmt.Errorf("...: %w", err) 包装过，
// == 只比对最外层，包装过的错误永远判不出来
if err == context.DeadlineExceeded { ... }

// 正确：errors.Is 会沿 Unwrap 链逐层比对
if errors.Is(err, context.DeadlineExceeded) { ... }
if errors.Is(err, context.Canceled) { ... }
```

💡 补充一个容易混淆的点：`ctx.Err()` 本身**不会返回被包装的错误**——它要么是 `nil`，要么就是 `context.Canceled` 或 `context.DeadlineExceeded` 这两个哨兵值，所以单独写 `ctx.Err() == context.DeadlineExceeded` 是能工作的。但业务代码里拿到的通常是**调用链层层包装后的 `err`**，那时就必须用 `errors.Is`。为了不给读者留下"有时 `==` 也行"的印象，统一用 `errors.Is` 是最稳妥的写法。

原因是业务代码里错误几乎总是被 `fmt.Errorf("...: %w", err)` 包装过好几层，从最内层的数据库驱动到最外层的 HTTP 中间件，每一层都加了自己的上下文。`errors.Is` 会沿 `Unwrap` 链逐层比对，`==` 只比对最外层。11.1.10 的完整链路示例会看到：最终拿到的错误文本是 `orderService.Handle: callRPC: context deadline exceeded`，但 `errors.Is(err, context.DeadlineExceeded)` 依然是 `true`。

🔥 重点：区分"我的超时"和"上游的取消"在业务上非常重要。前者说明下游太慢，应该告警、降级、扩容量；后者说明调用方不感兴趣了，属于正常现象，不该计入错误率。两者都表现为"调用失败"，但只有 `errors.Is` 能把它们分开。

### 11.1.6 Done() 的三种消费方式

**方式一：`select` 等待（最常用）**。下面三种消费方式都是片段，只保留关键几行：

```go
select {
case <-ctx.Done():
	return ctx.Err()
case v := <-work:
	handle(v)
}
```

**方式二：在发送/接收时"带取消"地操作**，这是防止 goroutine 卡死的关键：

```go
select {
case out <- v:      // 下游可能在无人接收
case <-ctx.Done():  // 一旦取消，立刻放弃发送
	return
}
```

如果少了 `<-ctx.Done()` 这一路，当上游取消、下游也不再读取时，这个 goroutine 会永久阻塞在 `out <- v` 上，成为泄漏。本章 11.4 的每个模式都会在"发送"和"接收"两侧同时挂上取消分支——看起来啰嗦，但这是生产级代码与玩具代码的分界线。

**方式三：`context.AfterFunc` 注册清理回调（Go 1.21+）**，适合"取消后要关连接、删临时文件"这类场景，见 11.1.10。

### 11.1.7 WithValue：能不用就不用

`WithValue` 是 `context` 里最容易被滥用的 API。它的设计意图非常窄：**只放请求域的、横切关注点的元数据**，典型的就是 traceID、userID、租户 ID、语言/时区这类"每一层都可能要、但又不值得作为参数显式传递"的信息。

先看正确用法：

```go
// 摘自本章验证用例 a02_ctx_value/main.go
package main

import (
	"context"
	"fmt"
)

// ctxKey 定义为包内未导出的自定义类型，其他包无法构造同名键，
// 从根本上杜绝了键冲突。
type ctxKey int

const (
	keyTraceID ctxKey = iota
	keyUserID
)

// withTraceID 是往 Context 里塞请求域元数据的唯一入口，
// 业务代码只依赖它，不直接接触 WithValue。
func withTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, keyTraceID, traceID)
}

// traceIDFrom 取值时永远做带 ok 的类型断言，断言失败返回零值。
func traceIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(keyTraceID).(string)
	return id
}

func userIDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(keyUserID).(int64)
	return id
}

// handle 只接收 ctx，不接收 traceID 参数：
// 跨层传递的请求域数据走 Context，业务参数走显式参数。
func handle(ctx context.Context) {
	fmt.Printf("handle: traceID=%q userID=%d\n", traceIDFrom(ctx), userIDFrom(ctx))
}

func repository(ctx context.Context) {
	// 底层函数无需感知 traceID 是怎么传下来的，直接取用即可
	fmt.Printf("repository: traceID=%q\n", traceIDFrom(ctx))
}

func main() {
	ctx := withTraceID(context.Background(), "trace-8f2a")
	ctx = context.WithValue(ctx, keyUserID, int64(10086))

	handle(ctx)
	repository(ctx)

	// 子 Context 可以遮蔽父 Context 的同名键，查找沿链向上直到命中
	child := withTraceID(ctx, "trace-child")
	fmt.Println("子 ctx:", traceIDFrom(child), "父 ctx:", traceIDFrom(ctx))

	// 键不存在时返回 nil，必须做断言保护
	fmt.Println("缺失的键:", ctx.Value(keyUserID) == nil, ctx.Value("不存在的键") == nil)

	// 反例演示：内置 string 做键，任何包都能写出同样的 "uid"，互相遮蔽
	bad := context.WithValue(context.Background(), "uid", "包 A 写入的用户")
	bad = context.WithValue(bad, "uid", "包 B 写入的用户")
	fmt.Println("字符串键被遮蔽:", bad.Value("uid"))
}
```

运行结果：

```text
handle: traceID="trace-8f2a" userID=10086
repository: traceID="trace-8f2a"
子 ctx: trace-child 父 ctx: trace-8f2a
缺失的键: false true
字符串键被遮蔽: 包 B 写入的用户
```

解读：

- **键必须是自定义类型，绝不要用内置 `string` 或 `int`。** 最后一行演示了后果：两个包都想用 `"uid"` 做键，后写入的会遮蔽先写入的，而 `go vet`、编译器都不会报错，只有线上数据错乱时你才发现。把键定义成包内未导出的 `type ctxKey int`，其他包连构造同名键的能力都没有。
- **值必须做带 `ok` 的类型断言。** `Value` 返回 `any`，取一个不存在的键会得到 `nil`，直接断言成 `string` 会 panic。把取值封成一个 `traceIDFrom` 函数，业务代码就永远不用关心这个细节。
- **读写都收口成一对函数。** 调用方只看到 `withTraceID` / `traceIDFrom`，将来换成别的实现（比如换成 `otel` 的 span）时改动面极小。
- **子 Context 可以遮蔽父 Context 的同名键。** 查找沿链向上，命中即返回，这既是特性（允许局部覆盖）也是坑（调试时容易看错层）。

再列一遍**不该放进 Context 的东西**，这几条在代码评审里非常常见：

| 反模式 | 为什么错 | 正确做法 |
| --- | --- | --- |
| 把可选业务参数放进 Context | 函数签名看不出依赖，调用方永远不知道要传什么；IDE 与编译器都帮不了你 | 显式参数、选项模式（functional options）、配置结构体 |
| 把数据库连接、logger、缓存放进去 | Context 会跨 API 边界传播，等于把基础设施泄漏到每一层，测试时无法替换 | 构造函数注入字段 |
| 往 Context 里塞可变的大对象 | Context 会随派生链一直被引用，活多久取决于最长的那个子节点 | 只放不可变的、体积小的元数据 |
| 用 Context 当"全局变量"传递状态 | 与显式数据流背道而驰，无法静态分析，并发下更难推理 | 显式传参或用依赖注入 |

💡 提示：如果某个值"每一层都要用，而且换了实现也不影响业务语义"，它才配放进 Context。判断标准可以简化成一句话：**把它删掉重新编译，如果很多层都编译不过，说明它本该是显式参数。**

### 11.1.8 不可变性与并发安全

Context 有两个重要的保证：

1. **不可变**：没有任何 API 能修改一个已存在的 Context。`WithValue(ctx, k, v)` 不会"改" `ctx`，而是返回一个包着 `ctx` 的新节点。因此你可以放心地把同一个 Context 交给任意多个 goroutine 共享，无需加锁。
2. **并发安全**：`Done()`、`Err()`、`Value()` 都可以被任意多个 goroutine 并发调用；`cancel()` 也可以被并发调用（幂等）。

但"Context 安全"**不等于**"放在里面的东西安全"。`WithValue(ctx, key, myMap)` 中的 `myMap` 仍然需要你自己保证并发安全——Context 只保证它自己那层包装是安全的。同理，Context 只负责广播取消信号，**它不会中断正在执行的函数**：一个 `for {}` 死循环不会因为 `ctx.Done()` 关闭而停下，必须由你自己在循环里检查。

### 11.1.9 使用规范：8 条铁律

这 8 条是 Go 社区（以及官方文档）反复强调的约定，建议逐条对照自己的代码：

1. **Context 作为函数的第一个参数**，命名为 `ctx`：`func Do(ctx context.Context, arg T) error`。放在第一位是纯约定，但所有 Go 程序员都默认如此，破坏它会让代码读起来很别扭。
2. **不要把它塞进结构体**。结构体是"持久的状态"，Context 是"一次调用的生命周期"，两者的生存期根本不同。
3. **不要传 `nil`**。`nil` 会让下游的 `ctx.Done()` 直接 panic。不确定用什么就传 `context.TODO()`——它在语义上大声宣告"这里还没决定"。
4. **只在函数内部派生，不在里面长期持有**。派生出的子 Context 的生命周期应该和函数调用一致。
5. **取消函数必须调用**，优先 `defer cancel()`。
6. **不要用 Context 传可选参数或业务对象**（见上表）。
7. **不要把它当 logger / DB 句柄的容器**。
8. **中间层要透传，不要偷偷换掉**。中间层若确实需要更短的超时，用 `WithTimeout` 派生一个子 Context，而不是把上游的 ctx 丢掉换成 `Background()`——后者是"超时链路断裂"的头号原因。

第 8 条尤其值得展开成代码，因为它正是下一节实战要讲的东西。

### 11.1.10 超时传播实战：一条链路的完整预算

下面这个例子模拟 HTTP 入口 → 业务层 → 数据库 → 下游 RPC 的完整链路。请重点关注：**只有入口处设了一次超时**，之后所有层都是透传。

```go
// 摘自本章验证用例 a03_ctx_chain/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// queryDB 模拟数据库查询：把 ctx 透传给驱动层，驱动层会在 ctx 到期时中断查询。
func queryDB(ctx context.Context, sql string) (string, error) {
	select {
	case <-time.After(10 * time.Millisecond):
		return "db(" + sql + ")", nil
	case <-ctx.Done():
		return "", fmt.Errorf("queryDB: %w", ctx.Err())
	}
}

// callRPC 模拟下游 RPC：本例中它比总超时预算还慢，用来触发超时。
func callRPC(ctx context.Context, req string) (string, error) {
	select {
	case <-time.After(200 * time.Millisecond):
		return "rpc(" + req + ")", nil
	case <-ctx.Done():
		return "", fmt.Errorf("callRPC: %w", ctx.Err())
	}
}

type orderService struct{}

// Handle 是业务层，它不设置新的超时，只透传上游 ctx。
func (s *orderService) Handle(ctx context.Context, id string) (string, error) {
	detail, err := queryDB(ctx, "select * from orders where id="+id)
	if err != nil {
		return "", fmt.Errorf("orderService.Handle: %w", err)
	}
	score, err := callRPC(ctx, "risk/"+id)
	if err != nil {
		return "", fmt.Errorf("orderService.Handle: %w", err)
	}
	return detail + " " + score, nil
}

// handleHTTP 模拟 HTTP 入口：整个请求只有一个总预算，所有下游共享它。
func handleHTTP(parent context.Context, id string) {
	ctx, cancel := context.WithTimeout(parent, 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := (&orderService{}).Handle(ctx, id)
	if err != nil {
		fmt.Printf("请求失败, 耗时 %v, 错误链: %v\n", time.Since(start).Round(time.Millisecond), err)
		fmt.Println("判定为超时:", errors.Is(err, context.DeadlineExceeded))
		return
	}
	fmt.Println("请求成功:", res)
}

func main() {
	// 所有请求共享同一个父 Context（在真实服务里它由 http.Server 注入）
	root := context.Background()
	handleHTTP(root, "1001")

	// 客户端提前断开：父 Context 取消会级联到所有子 Context
	ctx, cancel := context.WithCancel(root)
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel() // 相当于客户端断连
	}()
	start := time.Now()
	_, err := (&orderService{}).Handle(ctx, "1002")
	fmt.Printf("客户端断开, 耗时 %v, 错误: %v\n", time.Since(start).Round(time.Millisecond), err)
	fmt.Println("判定为取消:", errors.Is(err, context.Canceled))
}
```

运行结果：

```text
请求失败, 耗时 50ms, 错误链: orderService.Handle: callRPC: context deadline exceeded
判定为超时: true
客户端断开, 耗时 20ms, 错误: orderService.Handle: callRPC: context canceled
判定为取消: true
```

（第二行的耗时是 `time.Since(...).Round(time.Millisecond)` 的结果，`cancel()` 在 20 毫秒处发出，加上错误向上返回的时间，**多次运行会看到 20ms 或 21ms**，属于正常的测量抖动；第一行的 50ms 由 `WithTimeout` 决定，是稳定的。）

解读：

- **超时预算只设一次**。入口处 `context.WithTimeout(parent, 50*time.Millisecond)` 决定了整条链路的生死，业务层、DB 层、RPC 层都不再自设超时。这样"整条请求用了多久"是可预期的，而不是各层超时相乘（50ms + 200ms + ...）导致长尾失控。
- **每一层都用 `%w` 包装错误**，所以最终错误链里能看到完整路径 `orderService.Handle: callRPC: context deadline exceeded`，用 `errors.Is` 又能精确识别出最内层的原因。
- **父 Context 取消时子链路会秒退**。第二个场景里 `cancel()` 相当于客户端断连，业务层在 20 毫秒处就返回了，而不是傻等到 50 毫秒的超时——这就是"级联取消"的价值，也是节省后端资源的关键。
- **谁发起，谁设超时**。真实项目里通常由 HTTP 中间件统一注入（`http.Server` 已经能让 `r.Context()` 感知客户端断开），gRPC 则通过 metadata 自动传递 deadline。

### 11.1.11 Go 1.20 / 1.21 新增的四个 API

`context` 包在 Go 1.20 和 1.21 补上了几个长期缺失的能力，用得好能省掉不少手写胶水。

**`context.WithCancelCause` + `context.Cause`（Go 1.20）**：让取消携带业务原因。`ctx.Err()` 永远只返回 `context.Canceled`，而 `context.Cause(ctx)` 能拿到你传进去的那个 error，这在排查"到底是谁取消的"时非常有用。

**`context.WithTimeoutCause` / `WithDeadlineCause`（Go 1.21）**：超时也可以带原因，比如"下游 RPC 太慢"。

**`context.WithoutCancel`（Go 1.21）**：派生一个"脱离取消、保留 Value"的 Context。最典型的场景是**异步写审计日志**：请求已经取消、响应已经返回，但日志必须写成功；同时又需要日志里带着 traceID。

**`context.AfterFunc`（Go 1.21）**：注册一个"Context 取消后异步执行"的回调，返回值 `stop` 可以撤销注册。它比在 goroutine 里 `<-ctx.Done()` 更省事，也不会在 Context 一直不取消时泄漏一个等待协程。

```go
// 摘自本章验证用例 a04_ctx_newapi/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ctxKey int

const keyUserID ctxKey = iota

var errUserLogout = errors.New("用户主动登出")

func main() {
	base := context.WithValue(context.Background(), keyUserID, int64(42))

	// 1) WithCancelCause + context.Cause：给取消带上业务原因
	ctx, cancel := context.WithCancelCause(base)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel(errUserLogout)
	}()
	<-ctx.Done()
	fmt.Println("Err():", ctx.Err())            // 永远是 Canceled
	fmt.Println("Cause():", context.Cause(ctx)) // 业务原因
	fmt.Println("errors.Is(Cause):", errors.Is(context.Cause(ctx), errUserLogout))

	// 2) WithTimeoutCause：超时也可以携带原因
	tctx, tcancel := context.WithTimeoutCause(
		context.Background(), 20*time.Millisecond, errors.New("下游 RPC 太慢"))
	defer tcancel()
	<-tctx.Done()
	fmt.Println("超时 Err:", tctx.Err(), "/ 超时 Cause:", context.Cause(tctx))

	// 3) WithoutCancel：保留 Value，但切断取消信号（用于异步写审计日志等场景）
	detached := context.WithoutCancel(ctx)
	fmt.Println("detached.Done() == nil:", detached.Done() == nil)
	fmt.Println("detached 仍带着 userID:", detached.Value(keyUserID))
	select {
	case <-detached.Done():
		fmt.Println("不应该走到这里")
	default:
		fmt.Println("detached 不会被父 ctx 取消")
	}

	// 4) AfterFunc：注册一个在 Context 取消后异步执行的回调，
	// 返回值 stop 用于在取消之前撤销注册。
	ctx2, cancel2 := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx2, func() { fmt.Println("AfterFunc: 执行清理逻辑") })
	cancel2()
	time.Sleep(50 * time.Millisecond) // 等待回调执行完，仅演示用
	fmt.Println("回调已触发，stop() 返回:", stop())

	// 5) AfterFunc 也可以用来实现"先到先得"的等待
	ctx3, cancel3 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel3()
	<-ctx3.Done()
	fmt.Println("ctx3 已到期:", errors.Is(ctx3.Err(), context.DeadlineExceeded))
}
```

运行结果：

```text
Err(): context canceled
Cause(): 用户主动登出
errors.Is(Cause): true
超时 Err: context deadline exceeded / 超时 Cause: 下游 RPC 太慢
detached.Done() == nil: true
detached 仍带着 userID: 42
detached 不会被父 ctx 取消
AfterFunc: 执行清理逻辑
回调已触发，stop() 返回: false
ctx3 已到期: true
```

解读：

- `ctx.Err()` 与 `context.Cause(ctx)` 的分工要记牢：**判类型用 `Err()` + `errors.Is`，查原因用 `Cause()`**。当父节点没有设置 cause 时，`Cause()` 会退化成和 `Err()` 一样的值（`Canceled` 或 `DeadlineExceeded`），所以可以放心地在日志里统一打印 `Cause()`。
- `WithoutCancel` 返回的 Context `Done()` 为 `nil`，在 `select` 里等于"永不触发"；但 `Value` 链完整保留。**它的风险也在这里**：脱敏取消意味着这段代码不再受任何超时保护，必须自己加超时，否则异步任务可能永远挂住。
- `AfterFunc` 的回调在**独立 goroutine** 中执行（官方文档明确说明），因此回调内部的 panic 不会被你的 `recover` 兜住，回调里做的事要尽量简单可靠。`stop()` 返回 `false` 表示"回调已经开始跑或已经跑完了"，此时无法再撤销。

## 11.2 Go 调度器 GMP 模型

### 11.2.1 三个角色：G、M、P

Go 运行时用自己的调度器取代了操作系统的线程调度，核心是三个结构体：

| 角色 | 全称 | 是什么 | 谁创建 | 数量级 |
| --- | --- | --- | --- | --- |
| G | Goroutine | 用户态协程：栈、状态、指令位置 | `go` 关键字、运行时内部 | 可达百万级 |
| M | Machine | 内核线程（OS thread），真正被 CPU 执行的实体 | 运行时按需创建 | 通常几十个，默认上限 10000 |
| P | Processor | 逻辑处理器，是"运行 G 所需的资源"的抽象：本地队列、内存缓存、调度上下文 | 启动时按 GOMAXPROCS 创建 | 固定为 GOMAXPROCS 个 |

一条最容易被忽略、也最能解释调度行为的规则是：**M 必须持有一个 P 才能执行 G**。所以"并行度"实际上由 P 的数量（也就是 `GOMAXPROCS`）决定，而不是由 M 或 G 的数量决定。你可以起 100 万个 goroutine，但同一时刻真正在 CPU 上跑的只有 `GOMAXPROCS` 个。

```text
                 全局运行队列 GRQ（有锁保护）
        +---------------------------------------------+
        |  G   G   G   G   G   ...                     |
        +---------------------------------------------+
              ^                      |
   偷不到时批量放回              每 61 次调度取一批
              |                      v
   +-------------------+   +-------------------+   +-------------------+
   |        P0         |   |        P1         |   |        P2         |
   |  runnext --> G    |   |  runnext --> G    |   |  runnext --> nil  |
   |  runq [256] 本地队列| |  runq [256]       |   |  runq [256]       |
   |  [G G G G G]      |   |  [G G]            |   |  []               |
   |  mcache / 调度上下文|  |                   |   |                   |
   +-------------------+   +-------------------+   +-------------------+
            |                       |                       |
            | 绑定                   | 绑定                   | 绑定
            v                       v                       v
        +-------+               +-------+               +-------+
        |  M0   |               |  M1   |               |  M2   |  <- 内核线程
        +-------+               +-------+               +-------+
            |                       |                       |
        ~~~~~~~~~~~~~~~~~ 内核调度器 / CPU 核心 ~~~~~~~~~~~~~~~~~
```

### 11.2.2 本地队列与全局队列：为什么要有两个

每个 P 都有一个私有的本地运行队列（Local Run Queue, LRQ），**容量 256**，还有一个只有一个槽位的 `runnext`；此外全进程还有一个全局运行队列（Global Run Queue, GRQ）。

为什么不全用一个队列？答案是**锁竞争与局部性**：

- 如果只有一个全局队列，每次 `go f()` 和每次调度都要抢同一把锁。在 32 核机器上，这把锁会迅速成为瓶颈。
- 分成 P 私有的 LRQ 之后，绝大多数入队/出队操作**完全无锁**（只有该 P 自己访问自己的队列）。
- `runnext` 是一个特殊的"插队位"：最新创建的 goroutine 会被放到这里，并且**优先于本地队列里已有的 G 执行**。原因很实际——`go f()` 之后紧跟的往往是 `f` 依赖的数据，立刻执行它能吃到 CPU 缓存的热数据；同时这能让 `go f(); close(done)` 这类紧随其后的协作更顺滑。
- 本地队列满了（超过 256）怎么办？运行时会把当前 P 的本地队列**搬一半**到全局队列，避免某个 P 撑爆而其他 P 闲着。

### 11.2.3 调度循环：schedule → execute → goexit

一个 M 在生命期内反复执行同一个循环（源码在 `runtime/proc.go`）：

```text
   go f()  -->  newproc  -->  放入当前 P 的 runnext（被挤出的旧值进入 LRQ 尾部）
                                    |
                                    v
   +---------------- M 的调度循环 ----------------+
   |                                             |
   |  schedule() 依次尝试：                        |
   |    1) 每 61 次调度，去 GRQ 取一批（避免饿死）    |
   |    2) runnext                               |
   |    3) 自己的 LRQ 队头                        |
   |    4) work stealing：随机挑一个 P，偷它一半     |
   |    5) 再试 GRQ，最后 netpoll（就绪的网络事件）   |
   |    6) 实在没活：M 自旋等待，或挂起休眠          |
   |                    |                        |
   |                    v                        |
   |  execute(g) --> gogo：切换栈，开始执行 G       |
   |                    |                        |
   |                    v                        |
   |  G 结束 --> goexit()：清理栈、把 G 放回空闲池，  |
   |                      回到 schedule()          |
   +---------------------------------------------+
```

三个细节值得记住：

1. **第 1 步的"每 61 次"**（源码里是 `pp.schedtick%61 == 0`）是为了公平：如果只从本地队列取，全局队列里的 G 可能被饿死。
2. **自旋（spinning）**：当有可运行的 G 却没有空闲 P 时，运行时会让少量 M 空转（通常不超过忙碌 P 的一半），以便有 G 完成时能立刻接手，避免陷入内核睡眠再唤醒的延迟。
3. **G 结束后并不销毁**：它的栈和结构体会被 P 缓存（`gFree`），下次 `go` 直接复用，这就是"goroutine 很便宜"的底气——一个 goroutine 初始只需要 2 KB 左右的栈。

### 11.2.4 work stealing 与 hand off

这两个机制解决的是两个相反的问题：

- **work stealing（工作窃取）**：某个 P 的队列空了，而别的 P 还有一堆活。空转的 M 会随机挑选一个受害者 P，**偷走它一半的 G**（源码注释原话是 "Steal half of elements from local runnable queue"）。随机挑选是为了避免所有 P 都去抢同一个队列。
- **hand off（移交）**：某个 M 因为系统调用阻塞了，但它手里的 P 不能跟着一起睡——P 是稀缺资源，必须转给别人。运行时会把 P 交给另一个 M（唤醒一个空闲 M 或新建一个），让这个 P 继续跑队列里的其他 G。

```text
   M0 正在跑 G1，G1 发起阻塞式系统调用（如 read 一个慢文件）
        |
        v
   +---------------------+    hand off    +------------------------+
   | M0: 进入 syscall 状态 |  ----------->  | P0 被移交给空闲的 M3    |
   |      手里仍然"记着"P0 |                | M3 立刻接管 P0          |
   +---------------------+                | 继续跑 P0 队列里的 G2   |
        |                                  +------------------------+
        | syscall 返回
        v
   回到用户态，M0 尝试"抢回"一个 P：
        |-- 抢到了  --> 把 G1 挂到该 P 上继续跑
        |-- 抢不到  --> 把 G1 放入 GRQ，M0 自己休眠或退出
```

⚠️ 注意：这就是"阻塞系统调用很贵"的根本原因。一次阻塞调用至少涉及一次 P 的移交和一次线程唤醒，成本远高于 goroutine 切换。所以 Go 把网络 I/O、文件 I/O 大量实现为**非阻塞 + netpoller**（运行时用 epoll/kqueue/IOCP 统一管理），让"等待"表现为"G 被挂起"，而不是"线程被阻塞"。这也是为什么用 Go 写高并发网络服务时，同步风格的代码也能有很高的吞吐。

### 11.2.5 sysmon：调度器的"监工"

运行时还有一个独立的后台线程 `sysmon`（system monitor），它不持有 P，但负责几件关键的事：

- 检测长时间处于系统调用中的 M（超过约 20 微秒就有机会**抢占它的 P**）；
- 把长时间运行的 G 标记为可抢占，触发异步抢占（见下）；
- 触发强制 GC、归还长时间空闲的内存给操作系统；
- 轮询 netpoll，把就绪的网络 I/O 对应的 G 唤醒并放回队列。

理解了 sysmon，就能理解"为什么一个死循环不会永远占住 CPU 不放"。

### 11.2.6 抢占式调度：从协作到异步

Go 需要抢占，因为调度器必须能打断那些"不知道自己该让出 CPU"的代码。

**Go 1.14 之前是协作式抢占**。编译器会在函数的序言（prologue）处插入一段栈增长检查代码，运行时利用它作为"抢占检查点"：需要抢占时就把 `g.stackguard0` 设成一个特殊值，函数一进入就会踩到这个检查点，从而主动让出。这个方案有两个硬伤：

1. **没有函数调用的循环无法被抢占**。`for i := 0; i < n; i++ { x++ }` 这种循环里没有函数调用，序言检查永远不会执行，于是它会一直占着 P。更糟的是**这会导致 STW 卡死**：GC 需要所有 goroutine 停下来，而一个不带函数调用的死循环可以拖住整个 GC。
2. 抢占点稀疏，最坏情况下的延迟不可控。

**Go 1.14 引入了基于信号的异步抢占**。思路是：sysmon 发现某个 G 跑得足够久（默认约 10 毫秒），就给它所在的 M 发送一个信号（Unix 上是 `SIGURG`），运行时在信号处理函数里让当前 G 停下来、进入调度循环。这样即使是最纯粹的忙等循环也能被抢占，GC 的 STW 时间不再依赖代码形态。

🔥 重点：异步抢占是**以信号为载体的**，所以在 Windows、Linux、macOS 上都可用；但如果你在做 cgo 或自己屏蔽了信号的极端场景，抢占延迟会变差。实践含义是：**不要写没有函数调用的超长循环**，这既是性能问题，也是可抢占性问题。

### 11.2.7 goroutine 的栈为什么能一直长

Go 的 goroutine 使用**可增长的分段栈**（准确说是"连续栈，按需复制"），这是它能开几百万个的关键。

```text
   进入函数时先比较 SP 与 stackguard0：
        if SP < g.stackguard0  -->  调用 morestack

   morestack 做的事：
   +------------------+          +------------------+
   |  旧栈（2 KB）     |          |  新栈（4 KB）     |
   |  ...             |  复制 --> |  ...（原样拷贝）  |
   |  局部变量         |          |  局部变量         |
   +------------------+          +------------------+
             |                              ^
             |  扫描旧栈，凡是"指向栈内"的     |
             +--- 指针，全部改写成指向新栈 ----+
                 然后继续执行，调用方毫无感知
```

关键点：

- **初始栈很小**（约 2 KB，比线程默认栈 1~8 MB 小三个数量级），所以 goroutine 的"启动成本"极低。
- **栈按需翻倍增长，缩容时也会回收**（GC 期间如果栈用得很少，会被缩回小栈）。
- **栈是"可移动的"**，因为运行时会把旧栈内容整体复制到新栈，并且**修正所有指向栈内地址的指针**。这一点直接决定了两条编码规范：
  - 不要把 Go 对象的指针传给 C 代码长期保存（cgo 规则之一），因为栈一旦移动，C 侧保存的地址就失效了；
  - 不要依赖栈上变量的地址在 goroutine 生命期内保持稳定（虽然标准库与运行时保证"当前可见的指针一定被修正"，但把栈地址当作唯一标识（例如做 map 的 key）是很脆弱的做法）。
- **C 的线程栈不能增长**，因为 C 语言里到处都是指向栈的指针（甚至把栈地址存进全局变量或结构体），运行时无法安全地找出并改写它们。Go 之所以能这么做，是因为它有精确的栈映射（stack map）与垃圾回收器的扫描能力——**这是"编译器和运行时协作"换来的自由度**。

### 11.2.8 GOMAXPROCS：含义、容器与自动更新

`GOMAXPROCS` 的含义要说得精确：它限制的是**同一时刻可以执行用户 Go 代码的 P 的数量**，不是线程数，也不是 goroutine 数。当 G 进入系统调用或阻塞时，其他 G 仍然可以通过别的 P 继续运行。

关于它的默认值，有很长一段历史需要澄清：

- **Go 1.24 及更早**：默认值等于 `runtime.NumCPU()`，也就是机器上的逻辑 CPU 数。在容器里这是个灾难——一个限制 `--cpus=2` 的 Pod 跑在 64 核宿主机上，Go 程序会以为自己有 64 个核，开 64 个 P、大量线程，然后在 CPU 配额上疯狂被限流（throttle），延迟飙升。这就是 `uber-go/automaxprocs` 这类库存在的唯一理由：它在启动时读取 cgroup 配额并调用 `runtime.GOMAXPROCS()`。
- **Go 1.25 及以后**：运行时**原生感知容器**。默认值取以下三者的最小值：逻辑 CPU 数、进程的 CPU 亲和性掩码（affinity mask）、以及 Linux cgroup 的 CPU 吞吐限制（cgroup v2 读 `cpu.max`，v1 读 `cpu.cfs_quota_us` / `cpu.cfs_period_us`）。非整数的配额会向上取整；除非逻辑 CPU 或亲和性本来就小于 2，否则默认值不会低于 2。同时运行时**每秒最多更新一次**这个默认值，以跟随配额变化。
- 想关掉这套行为：`GODEBUG=containermaxprocs=0`（关闭容器感知，退回 `NumCPU`）或 `GODEBUG=updatemaxprocs=0`（关闭周期更新）。注意对语言版本 1.24 及更早的模块，这两个开关的默认值就是关闭状态。
- 一旦你**显式设置**了 `GOMAXPROCS` 环境变量或调用了 `runtime.GOMAXPROCS(n)`，自动更新就停止；想恢复默认行为可以调用 `runtime.SetDefaultGOMAXPROCS()`。

💡 提示：如果你的项目还在用较老的 Go 版本，或者需要"容器内再按业务自我限制"（例如一个 Pod 里跑多个服务），`automaxprocs` 仍然有价值；但在 Go 1.25+ 上它通常已经多余。另外要注意：**CPU limit 才是关键，CPU request 不影响**这一默认值的计算。

### 11.2.9 用 GODEBUG=schedtrace 观察调度器

调优的第一步是观测。运行时提供了零成本的追踪开关 `GODEBUG=schedtrace=N`，每 N 毫秒往标准错误打一行调度器快照。

```go
// 摘自本章验证用例 b01_schedtrace/main.go
package main

import (
	"runtime"
	"sync"
	"time"
)

func main() {
	// 只开 4 个 P，方便在 schedtrace 输出里数数
	runtime.GOMAXPROCS(4)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			// 纯 CPU 忙等 2.5 秒，制造可观的运行队列
			deadline := time.Now().Add(2500 * time.Millisecond)
			for time.Now().Before(deadline) {
			}
		})
	}
	wg.Wait()
}
```

用 `GODEBUG=schedtrace=1000 ./b01_schedtrace` 运行（Windows 上是 `set GODEBUG=schedtrace=1000` 后执行），真实输出如下（这是其中一次运行的快照：`gomaxprocs`、`threads`、`schedticks` 这些字段很稳定，实测同样得到 `[ 1 1 1 1 ]` → `[ 51 51 51 51 ]` → `[ 99 99 99 99 ]`；但**方括号里每个 P 的队列长度与 `runqueue` 会随采样时刻波动**，实测中也在 `[ 3 1 0 0 ]`、`[ 0 1 0 0 ]`、`[ 1 0 0 0 ]` 之间变化，不要把它们当成固定值）：

```text
SCHED 0ms: gomaxprocs=4 idleprocs=0 threads=6 spinningthreads=0 needspinning=1 idlethreads=0 runqueue=0 [ 3 1 0 0 ] schedticks=[ 1 1 1 1 ]
SCHED 1007ms: gomaxprocs=4 idleprocs=0 threads=6 spinningthreads=0 needspinning=1 idlethreads=0 runqueue=2 [ 1 1 0 0 ] schedticks=[ 51 51 51 51 ]
SCHED 2014ms: gomaxprocs=4 idleprocs=0 threads=6 spinningthreads=0 needspinning=1 idlethreads=0 runqueue=3 [ 0 0 1 0 ] schedticks=[ 99 99 99 99 ]
```

逐字段解读（字段名与 `runtime/proc.go` 的 `schedtrace` 函数一一对应）：

| 字段 | 含义 | 怎么用 |
| --- | --- | --- |
| `SCHED 0ms` | 距程序启动的毫秒数 | 对应时间轴 |
| `gomaxprocs=4` | P 的数量 | 与容器 CPU 配额是否匹配 |
| `idleprocs=0` | 空闲 P 数 | 长期等于 0 说明 P 全忙 |
| `threads=6` | 当前 M（内核线程）总数 | 异常增高往往意味着大量阻塞系统调用 |
| `spinningthreads=0` | 正在自旋找活的 M | 与 `needspinning` 配合看调度是否跟得上 |
| `needspinning=1` | 运行时认为"应该自旋"的 M 数 | 长期大于 0 说明有活儿没被及时接手 |
| `idlethreads=0` | 空闲 M 数 | 为 0 说明线程都被占用了 |
| `runqueue=0` | **全局**队列长度 | 非 0 说明本地队列装不下或发生了搬运 |
| `[ 3 1 0 0 ]` | **每个 P 的本地队列长度** | 分布是否均衡，是 work stealing 是否生效的直接证据 |
| `schedticks=[ ... ]` | 每个 P 累计的调度次数 | 各 P 数值差距大，说明负载倾斜 |

本例的解读：8 个纯 CPU 忙等 goroutine 挤在 4 个 P 上，`idleprocs=0`、`runqueue` 在 0~3 之间波动、四个 P 的队列长度此起彼伏——这正是 work stealing 在实时搬运任务的样子；`schedticks` 四个 P 基本同步增长（51、51、51、51 → 99、99、99、99），说明负载是非常均衡的。

想要更细的视图，加上 `scheddetail=1`：

```bash
# 每 1000 毫秒打一次概览，同时打印每个 P / M / G 的细节
GODEBUG=schedtrace=1000,scheddetail=1 ./app
```

它会额外打印 `gcwaiting`、`nmidlelocked`、`stopwait`、`sysmonwait` 等字段，以及每个 P 的 `status`、`schedtick`、`syscalltick`、绑定的 `m` 编号、`runqsize`、`gfreecnt`、`timerslen`；再往下是每个 M 的 `p`、`curg`、`mallocing`、`spinning`、`lockedg`，以及每个 G 的 `status`、`m`、`lockedm`、`waitreason`。排查"goroutine 到底卡在哪一类等待上"时，`waitreason` 是最有价值的字段之一（详见 11.6.2 的 `runtime/trace`）。

⚠️ 注意：`schedtrace` 会持有 `sched.lock` 打印，属于**观测工具而非生产常开开关**，只能在预发或临时排障时打开。

## 11.3 Go 内存模型与 happens-before

### 11.3.1 内存模型要解决什么问题

很多人以为"并发问题就是多个 goroutine 同时写一个变量"，于是想：只要写不重叠，应该就没事。现实要残酷得多。即使读写的是**不同的**变量，一个 goroutine 的写入也未必能被另一个 goroutine 及时看到，甚至未必按你写的顺序发生。原因有三层：

**第一层：编译器会重排序。** 只要不改变**单线程语义**，编译器就可以自由调整指令顺序。`a = 1; b = 2` 编译成 `b = 2; a = 1` 是完全合法的优化。

**第二层：CPU 会乱序执行。** 现代 CPU 为了填满流水线，会把没有依赖的指令并行执行甚至乱序提交。x86 的乱序程度相对温和，ARM/POWER 更激进。

**第三层：多核缓存与写缓冲。** 每个核心有自己的 store buffer 和 L1 缓存。写入先进 store buffer，稍后才写回缓存；两个核心的缓存通过一致性协议同步，但同步有延迟。

```text
        CPU 核心 0                          CPU 核心 1
   +------------------+               +------------------+
   |  寄存器 / 执行单元 |               |  寄存器 / 执行单元 |
   +------------------+               +------------------+
   |  Store Buffer    |               |  Store Buffer    |
   |  [ done = true ] |               |  [ x = 42 ]      |
   +------------------+               +------------------+
   |  L1 缓存          |               |  L1 缓存          |
   +------------------+               +------------------+
             \                                /
              \____ 缓存一致性协议（MESI 等）___/
                          |
              +--------------------------+
              |     主内存 / L3 缓存      |
              +--------------------------+

   结果：核心 1 可能"先看到 done = true"，却仍然读到旧的 x，
        因为两次写入经由不同的缓冲路径，可见顺序不可控。
```

内存模型（Memory Model）就是一份"什么情况下一个 goroutine 的写入能被另一个 goroutine 看到"的契约。Go 的内存模型给出的答案只有一句话：**用 happens-before 来描述同步**。

### 11.3.2 happens-before 的定义

官方定义（`go.dev/ref/mem`）：如果事件 e1 happens before 事件 e2，那么 e2 一定能观察到 e1 的效果。更准确地说：

- 如果两个事件之间**存在** happens-before 关系，那么它们的顺序是确定的；
- 如果两个事件之间**不存在** happens-before 关系，那么它们的顺序是**未定义的**：编译器、CPU、运行时都可以任意安排，程序的行为不可预测。

注意一个容易误解的点：happens-before **不等于"时间上先发生"**。它是一条**逻辑边**，由同步原语建立。没有同步原语的"物理上先发生"（比如 A 线程 10 毫秒写的值，B 线程 1 秒后才读）在语言层面**没有任何保证**。

Go 内存模型给出的 happens-before 建立规则可以整理成下面这张表（这是本章最值得记住的一张表）：

| 场景 | happens-before 关系 |
| --- | --- |
| 单个 goroutine 内部 | 语句按程序顺序执行（读操作能观察到同 goroutine 中在它之前的写） |
| 包的初始化 | 包 `p` 的 `init` 完成 → 引用 `p` 的包的 `init`/`main` 开始 |
| goroutine 的启动 | `go` 语句本身 → 新 goroutine 的执行开始 |
| goroutine 的退出 | **没有任何保证**：一个 goroutine 退出不意味着任何事情被同步 |
| channel 发送/接收（带缓冲） | 第 k 次发送中"值的发送完成" → 第 k 次接收中"值的接收完成" |
| channel 接收/发送（带缓冲） | 第 k 次接收中"值的接收完成" → 第 k+C 次发送的完成（C 为缓冲容量） |
| 无缓冲 channel | 接收完成 → 对应的发送完成 |
| channel 关闭 | `close(ch)` → 因为关闭而收到零值的接收 |
| `sync.Mutex` / `RWMutex` | 第 n 次 `Unlock` → 第 m 次 `Lock`（m > n） |
| `sync.Once` | `once.Do(f)` 中 f 的返回 → 任意一次 `once.Do(f)` 的返回 |
| `sync.WaitGroup` | `f`（由 `Go` 启动的函数）的返回 → 被它唤醒的那次 `Wait` 的返回 |
| `sync/atomic` | 若原子操作 A 的效果被原子操作 B 观察到，则 A → B；且**所有原子操作表现为某个全局的顺序一致顺序** |

几条最容易被忽略的推论：

1. **goroutine 退出不同步**。所以"起一个 goroutine 写全局变量，主 goroutine 睡 1 秒再读"是彻头彻尾的错误——尽管它"看起来能工作"。
2. **`WaitGroup.Wait` 返回**之后仍然要小心：只有在 `Wait` 被"这次 Done 唤醒"的语义下，被等待函数的写入才可见；如果 `Add` 与 `Wait` 的配对本身错了（比如在 `Wait` 之后才 `Add`），同步关系不成立。
3. **channel 的同步是"按第 k 次"计数的**。带缓冲容量为 C 的 channel，第 k 次接收只与第 k+C 次发送建立关系——这就是"缓冲 channel 不保证接收方看到最新数据"的来源。
4. **`close` 是广播式的同步点**：它 happens before 所有"因关闭而返回零值"的接收，因此可以安全地用它来发布一份只读数据（11.4.2 的 fan-in 就依赖这一点）。

### 11.3.3 违反 happens-before：一个不会退出的循环

来看本章验证用例里那段故意写错的代码：

```go
// 摘自本章验证用例 c01_visibility/main.go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	// 反例：用裸 bool 变量跨 goroutine 传递"我干完了"这件事。
	// 真正的业务代码这里会写成 for !done {}，那就是一个行为未定义、
	// 可能永不退出的忙等循环；本示例加了时间上界，只为保证能正常退出。
	var done bool
	go func() {
		time.Sleep(20 * time.Millisecond)
		done = true // 写：没有任何同步
	}()
	deadline := time.Now().Add(200 * time.Millisecond)
	for !done && time.Now().Before(deadline) { // 读：没有任何同步
	}
	fmt.Println("[反例] 裸变量观测到写入:", done, "（语义未定义，go run -race 会报 DATA RACE）")

	// 正确写法一：channel 既传递数据，又建立 happens-before
	ch := make(chan struct{})
	var payload string
	go func() {
		payload = "由子 goroutine 写入的载荷" // 写载荷
		close(ch)                      // close 之前的所有写入对读到关闭的一方可见
	}()
	<-ch
	fmt.Println("[channel] payload =", payload)

	// 正确写法二：atomic 的顺序一致性保证
	var flag atomic.Bool
	var data string
	go func() {
		data = "在 Store 之前写入的数据"
		flag.Store(true) // 释放语义
	}()
	for !flag.Load() { // 获取语义：一旦读到 true，上面的 data 写入必定可见
		time.Sleep(time.Millisecond)
	}
	fmt.Println("[atomic] data =", data)

	// 正确写法三：Mutex 的 Unlock 先于后续的 Lock
	var mu sync.Mutex
	var counter int
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		// WaitGroup.Go 内部自带 Add(1) 与 Done()，不要再手工 Add
		wg.Go(func() {
			mu.Lock()
			counter++
			mu.Unlock()
		})
	}
	wg.Wait()
	fmt.Println("[mutex] counter =", counter)
}
```

运行结果：

```text
[反例] 裸变量观测到写入: true （语义未定义，go run -race 会报 DATA RACE）
[channel] payload = 由子 goroutine 写入的载荷
[atomic] data = 在 Store 之前写入的数据
[mutex] counter = 100
```

解读：

- **反例为什么危险**：`done` 的读写之间没有任何 happens-before。编译器可以把 `done` 缓存在寄存器里，于是 `for !done {}` 永远看不到变化——这是一个**真实的、能写出死循环的 bug**，而不是理论问题。示例里加了 `time.Now().Before(deadline)` 这个上界，只是为了让你能安全地跑一遍看到它；生产代码里的 `for !done {}` 就可能永远挂在那里，同时 CPU 占满一个核。
- **反例的修复方式有三种**，都对应上面表格里的一行：换 channel（顺便传递数据）、换 `atomic.Bool`（适合纯标志位）、换 mutex（适合保护一组数据）。
- **channel 版本为什么对**：`close(ch)` happens before 了"因关闭而收到零值"的 `<-ch`，所以 `payload` 的写在 `<-ch` 返回后**必然可见**。这里传递数据用的是变量，但同步靠的是 channel——这就是"用通信来共享内存"的字面含义。
- **atomic 版本为什么对**：`flag.Store(true)` 被 `flag.Load()` 观察到，于是 A synchronizes before B，`data` 的写因此可见。注意这里没有传递任何数据，只传递了"顺序"。
- **mutex 版本为什么对**：第 n 次 `Unlock` happens before 第 m 次 `Lock`（m > n），因此每次 `counter++` 都在锁的临界区里，最终的 100 是确定的。

🔥 重点：**每一次读写共享变量，都要能指认出建立 happens-before 的那一行代码。** 指不出来，就说明这段代码是错的——不管它跑一万次都"看起来正常"。竞争条件的可怕之处正在于它不总是复现。

### 11.3.4 竞态检测器：把"不可复现"变成"每次都能复现"

Go 内置的 `-race` 是排查这类问题最有效的工具，它基于 ThreadSanitizer，在运行时动态检测"没有 happens-before 保护的并发访问"：

```bash
# 测试、运行、构建时都可以加 -race
go test -race ./...
go run -race ./cmd/server
go build -race -o app ./cmd/server
```

针对上面的反例，它会输出形如下面的报告（地址、行号、goroutine 编号因程序而异）：

> ⚠️ **关于下面的输出**：它是 ThreadSanitizer 报告的**格式示意**，不是在本机实测得到的——`-race` 需要 cgo 与 C 编译器（见下面第三个注意点），本书的验证环境没有 gcc，无法实际运行。**请读者在自己的机器上执行 `go run -race main.go` 对照**；真实报告通常还会在最后打印 `Found N data race(s)` 并以退出码 66 结束。

```text
==================
WARNING: DATA RACE
Read at 0x0000005c2a08 by main goroutine:
  main.main()
      /tmp/main.go:24 +0x88
Previous write at 0x0000005c2a08 by goroutine 7:
  main.main.func1()
      /tmp/main.go:20 +0x50
==================
```

报告的价值在于它同时给出了**读方、写方和两处的调用栈**，还带上建立不起来同步关系的事实：

⚠️ 注意：`-race` 有三个实际问题。第一，它会让程序变慢（通常数倍）并显著增加内存占用，所以只用于测试环境。第二，它**只能发现真正执行到的竞态**——没被触发的分支查不出来，因此覆盖率很重要。第三，Windows 上 `-race` 需要 C 工具链（MinGW-w64 的 gcc），否则会直接报 `cgo: C compiler "gcc" not found`；Linux 下则要求 cgo 可用。用不上 `-race` 时，至少要保证 CI 里有一台能跑它的机器——这项投入的回报率极高。

### 11.3.5 为什么 Go 的 atomic 比 C++ 的 relaxed 更强

C++ 的 `std::atomic` 提供了 `memory_order_relaxed`、`acquire`/`release`、`seq_cst` 等多档强度，而 Go 的 `sync/atomic` **只有一档**，等价于 C++ 的 `seq_cst`（也等价于 Java 的 `volatile`）。标准库文档的原话是：

> Additionally, all the atomic operations executed in a program behave as though executed in some sequentially consistent order. This definition provides the same semantics as C++'s sequentially consistent atomics and Java's volatile variables.

这意味着：

- **不存在"relaxed 原子操作"**。任何一次 `atomic.Load` / `atomic.Store` 都会参与全局的顺序一致顺序，不能与其他原子操作任意重排。
- **好处**：你几乎不可能写出"原子性对了但可见性错了"的代码，因为顺序一致天然覆盖了可见性；而且原子变量之间可以组合成无锁算法而不需要额外的 `fence`。
- **代价**：在弱内存模型架构（ARM、POWER）上，`seq_cst` 需要插入内存屏障指令，比 relaxed 慢；同时 Go 不给 `unsafe.Pointer` 之外的 atomic 提供"只保证原子性、不保证顺序"的廉价选项。

所以当你在 Go 里写 `atomic.Bool` 标志位时，可以放心地认为"Store 之前的写入，在 Load 看到 true 之后一定可见"；而在 C++ 里写 `relaxed` 就必须自己补 `fence`。

### 11.3.6 同步 = 原子性 + 可见性 + 顺序性

把这一节浓缩成一句必须记住的话：**同步不仅仅是"原子性"，它同时包含可见性与顺序性。**

- **原子性**：这个操作不会被撕裂，别人看到的是"全做"或"没做"（例如 64 位写不会只写一半）。
- **可见性**：这个写入什么时候能被其他 CPU 核心上的 goroutine 看到。
- **顺序性**：多个写入之间的先后次序，是否对其他 goroutine 一致。

`sync/atomic` 只解决原子性与顺序性，可见性由 happens-before 保证；裸变量一个都不解决。这也是为什么"我用 atomic 了呀"经常还是不生效——如果那个原子变量与你要保护的**数据**之间没有建立 happens-before，数据照样可能读到旧值。正确的组合永远是：**用同步原语保护你真正关心的数据，而不是只保护一个标志位。**

💡 提示：面试里常见的一个追问是"`sync.Mutex` 和 `atomic` 的差异"。标准答案不是"性能"，而是"能保护的东西不同"：`atomic` 只能保护单个字长的操作，任何"读—改—写"的复合逻辑（即使用了 CAS）都需要你自己处理重试与 ABA 问题；而 mutex 保护的是**一段临界区**，语义上更强也更容易用对。

## 11.4 并发模式大全

前面三节讲的是"原理"，这一节讲"招式"。下面 13 个模式覆盖了绝大多数工程场景，每个都给出**完整可运行的代码**、**适用场景**和**优缺点**。所有示例的目标版本是 Go 1.26，使用了 `golang.org/x/sync`、`golang.org/x/time` 的示例需要先执行：

```bash
go get golang.org/x/sync
go get golang.org/x/time
```

另外提前约定一条贯穿全部模式的纪律：**每个 goroutine 都必须有明确的退出路径，每个 channel 都必须由发送方关闭。** 下面每段代码都在刻意示范这两条。

关于代码的组织方式先说明一下：标注"**摘自本章验证用例 X/main.go**"的代码块都是**完整可运行的程序**（含 `package main` 与全部 import，已逐个通过 `go build`、`go vet` 并在 Go 1.26 实机运行，输出与正文给出的结果一致）；未做该标注的代码块是用于对照说明的**片段**，只展示关键几行，不能单独编译。这些完整示例也按文件名保存在本章配套代码目录里，可以直接拿来跑。

### 11.4.1 Generator：把数据源变成流

**适用场景**：把"产生数据"与"消费数据"解耦，尤其是数据量大到不适合一次性装进切片、或者生产速度与消费速度不匹配时。

```go
// 摘自本章验证用例 d01_generator/main.go
package main

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

// generator 把一组整数变成流：返回只读 channel，方向由类型系统固化，
// 调用方无法往里写，也无法关闭它。
func generator(ctx context.Context, nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out) // 发送方负责关闭
		for _, n := range nums {
			select {
			case out <- n:
			case <-ctx.Done(): // 下游提前退出时不会把上游永久挂死
				return
			}
		}
	}()
	return out
}

// take 只取前 n 个元素，取够就返回，剩下的交给 ctx 取消。
func take(ctx context.Context, in <-chan int, n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for range n { // Go 1.22+ 支持 range over int
			select {
			case v, ok := <-in:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func main() {
	base := runtime.NumGoroutine()

	nums := make([]int, 0, 1000)
	for i := 1; i <= 1000; i++ {
		nums = append(nums, i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := take(ctx, generator(ctx, nums...), 3)

	for v := range stream {
		fmt.Println("取到:", v)
	}
	cancel() // 通知上游停止；不取消的话 generator 会阻塞在发送上

	time.Sleep(20 * time.Millisecond)
	fmt.Printf("goroutine 数 %d -> %d（提前退出没有泄漏上游）\n", base, runtime.NumGoroutine())
}
```

运行结果：

```text
取到: 1
取到: 2
取到: 3
goroutine 数 1 -> 1（提前退出没有泄漏上游）
```

解读：

- **返回值是 `<-chan T`（只读）**，这是一道免费的编译期保护：调用方不可能误写、误关。生成器内部那个可写的 `out` 从未离开函数体。
- **`defer close(out)` 放在 goroutine 内部**，保证无论正常结束还是被取消，下游的 `for range` 都能退出。**关闭 channel 的责任永远在发送方**，这是唯一能避免"向已关闭 channel 发送"panic 的分工方式。
- **发送侧和接收侧都要挂 `ctx.Done()`**。只挂接收侧的生成器在下游提前退出时会永久阻塞在 `out <- n`；只挂发送侧的 take 会在上游"僵尸"（永不关闭也不发送）时卡死。
- 最后一行的 goroutine 计数回到 1，说明 `cancel()` 之后上游确实退出了。**这类"计数验证"值得写进单元测试**：它比人工 review 可靠得多。

💡 优缺点：优点是接口极简、惰性求值、天然支持无限流；缺点是多一层 goroutine 就多一次调度与上下文切换，纯 CPU 密集且数据总量很小的场景反而不如直接返回切片。

### 11.4.2 Fan-Out / Fan-In：并行处理与结果汇聚

**适用场景**：一批互相独立的任务需要并行处理，且结果需要统一汇总（批量调外部接口、并行压缩、并行计算）。

```go
// 摘自本章验证用例 d02_fanout_fanin/main.go
package main

import (
	"context"
	"fmt"
	"sync"
)

// gen 是数据源
func gen(ctx context.Context, nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			select {
			case out <- n:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// worker 是扇出的最小执行单元：从同一个 in 上竞争读取
func worker(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			select {
			case out <- v * v: // 模拟有耗时的计算
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// fanOut 启动 n 个 worker 并行消费 in，返回 n 个输出 channel
func fanOut(ctx context.Context, in <-chan int, n int) []<-chan int {
	outs := make([]<-chan int, n)
	for i := range n {
		outs[i] = worker(ctx, in)
	}
	return outs
}

// fanIn 把多个 channel 汇聚成一个，输出顺序不确定
func fanIn(ctx context.Context, chans ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup
	for _, c := range chans {
		// WaitGroup.Go 内部已经做了 Add(1) 和 Done()，不要再手工 Add/Done
		wg.Go(func() {
			for v := range c {
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		wg.Wait() // 所有上游都结束后才能关闭 out
		close(out)
	}()
	return out
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nums := make([]int, 0, 20)
	for i := 1; i <= 20; i++ {
		nums = append(nums, i)
	}

	// 一个数据源 -> 4 个 worker -> 汇聚成一个流
	merged := fanIn(ctx, fanOut(ctx, gen(ctx, nums...), 4)...)

	sum, count := 0, 0
	for v := range merged {
		sum += v
		count++
	}
	fmt.Println("元素个数:", count)
	fmt.Println("平方和:", sum) // 1^2+...+20^2 = 2870，与并发度无关
}
```

运行结果：

```text
元素个数: 20
平方和: 2870
```

解读：

- **fan-in 必须等所有上游都结束才能 `close(out)`**，所以需要一个 `WaitGroup` + 一个专职的 goroutine 做"关闭者"。把它写成两个 goroutine（一个 `wg.Wait()` 一个 `close`）是最常见的写法，注意 `close` 必须发生在 `wg.Wait()` 之后。
- **`WaitGroup.Go`（Go 1.25+）自带 `Add(1)` 与 `Done()`**，不要再手工 `Add`。手工加一次就会让计数器多出 n，`Wait` 直接永久阻塞。这个坑非常常见，务必记牢。
- **输出顺序不确定**：4 个 worker 竞争同一个输入队列，谁先算完谁先发。所以结果是 20 个元素、平方和 2870，但顺序是随机的。这一条必须在设计阶段就想清楚：**需要保序就必须带序号回填**，否则下游会得到不确定的结果。
- 并发度 `n` 的选择：CPU 密集取 `GOMAXPROCS` 左右，I/O 密集可以取几十甚至上百，但要注意下游（数据库、第三方接口）能承受的并发。

💡 优缺点：优点是实现简单、吞吐近似线性提升；缺点是输出无序、无法感知单个任务的失败（需要配合 11.5 的错误收集）、并发度固定。

### 11.4.3 Pipeline：多阶段流水线

**适用场景**：数据处理类任务（ETL、日志清洗、图片转码），每个阶段职责单一、处理速率不同，希望"边生产边消费"而不是分三步各自等待。

```go
// 摘自本章验证用例 d03_pipeline/main.go
package main

import (
	"context"
	"fmt"
	"time"
)

// 流水线的每个阶段都是同一个签名：吃一个流，吐一个流。
// 统一形状让阶段可以像积木一样拼接。
type stage func(context.Context, <-chan int) <-chan int

// genStage 是流水线头部：只吐不吃
func genStage(ctx context.Context, max int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 1; i <= max; i++ {
			select {
			case out <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// squareStage 平方，并模拟一点计算耗时
func squareStage(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out) // 每个阶段关闭自己的输出，这是流水线不泄漏的关键
		for v := range in {
			time.Sleep(time.Millisecond)
			select {
			case out <- v * v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// filterStage 只保留偶数
func filterStage(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			if v%2 != 0 {
				continue
			}
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// addStage 是带参数的高阶阶段，返回一个 stage
func addStage(delta int) stage {
	return func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for v := range in {
				select {
				case out <- v + delta:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}
}

// chain 按顺序套用多个阶段，返回最后一段流
func chain(ctx context.Context, src <-chan int, stages ...stage) <-chan int {
	for _, s := range stages {
		src = s(ctx, src)
	}
	return src
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// gen -> square -> filter(even) -> add(0)
	out := chain(ctx, genStage(ctx, 20), squareStage, filterStage, addStage(0))

	n := 0
	for v := range out {
		fmt.Println("结果:", v) // 4, 16, 36 ...
		n++
		if n == 3 {
			cancel() // 提前退出：上游阶段会在下一次 select 时看到 Done 并返回
			break
		}
	}
	fmt.Println("提前退出，共取到", n, "个结果")

	// 等待上游 goroutine 退出，验证没有卡死在发送上
	time.Sleep(30 * time.Millisecond)
	fmt.Println("流水线已停止")
}
```

运行结果：

```text
结果: 4
结果: 16
结果: 36
提前退出，共取到 3 个结果
流水线已停止
```

解读：

- **统一签名是流水线的灵魂**。所有阶段都是 `func(context.Context, <-chan T) <-chan T`，于是 `chain` 可以像搭积木一样把它们串起来，还能按需插拔、单独测试。
- **每个阶段只关闭自己的输出**，且关闭发生在该阶段的 goroutine 里。中间任何一环忘记 `close`，下游的 `for range` 就会永久阻塞——这是流水线最常见的死锁来源。
- **提前退出依赖 ctx**：消费者拿到前 3 个结果就 `cancel()`，上游阶段在下一次 `select` 时看到 `Done` 并退出，不会继续白算。注意 `time.Sleep` 那一步（模拟耗时）**不可被取消**，真实代码里应改成 `select { case <-time.After(d): case <-ctx.Done(): }`，本章的 `d12_retry` 就是这么写的。
- **收益与代价**：流水线让各阶段并行工作，吞吐由最慢的阶段决定；代价是多一层 channel 传递（每次大约几十纳秒量级的开销），阶段越细，协调成本越高。

💡 优缺点：优点是职责清晰、可组合、易测试、吞吐高；缺点是阶段数多时 goroutine 与 channel 数量上升，关停路径变复杂，必须统一用 ctx 管理生命周期。

### 11.4.4 Worker Pool：可动态扩缩的协程池

**适用场景**：任务量远超可承受的并发度、或单个任务会消耗数据库连接/文件句柄等稀缺资源时，用它把并发度钉死在一个可控范围。

```go
// 摘自本章验证用例 d04_workerpool/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var errTaskFailed = errors.New("任务执行失败")

// Task 是池子执行的任务单元，返回 error 表示业务失败。
type Task func(ctx context.Context) error

// Pool 是一个可动态扩缩的 worker 池。
type Pool struct {
	ctx    context.Context
	jobs   chan Task
	min    int32
	max    int32
	idle   time.Duration
	active atomic.Int32
	closed atomic.Bool

	mu   sync.Mutex
	wg   sync.WaitGroup
	errs []error
}

func NewPool(ctx context.Context, minWorkers, maxWorkers, queue int) *Pool {
	p := &Pool{
		ctx:  ctx,
		jobs: make(chan Task, queue),
		min:  int32(minWorkers),
		max:  int32(maxWorkers),
		idle: 60 * time.Millisecond,
	}
	p.scale(minWorkers)
	return p
}

// scale 增加 delta 个 worker，总数不超过 max。
func (p *Pool) scale(delta int) {
	for i := 0; i < delta; i++ {
		if p.active.Load() >= p.max {
			return
		}
		p.active.Add(1)
		p.wg.Go(p.worker) // wg.Go 内部自带 Add(1) 与 Done()
	}
}

func (p *Pool) worker() {
	defer p.active.Add(-1)

	idleTimer := time.NewTimer(p.idle)
	defer idleTimer.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case t, ok := <-p.jobs:
			if !ok { // 队列已关闭且已排空
				return
			}
			if !idleTimer.Stop() { // 停止计时器并安全排空
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(p.idle)
			if err := t(p.ctx); err != nil {
				p.record(err)
			}
		case <-idleTimer.C:
			// 空闲超时：只有在没有积压任务、且总数高于下限时才退役
			if len(p.jobs) > 0 || p.active.Load() <= p.min {
				idleTimer.Reset(p.idle)
				continue
			}
			return
		}
	}
}

func (p *Pool) record(err error) {
	p.mu.Lock()
	p.errs = append(p.errs, err)
	p.mu.Unlock()
}

// Submit 提交任务。队列满时会按需扩容，仍然满则阻塞等待。
func (p *Pool) Submit(t Task) error {
	if p.closed.Load() {
		return errors.New("worker pool 已关闭")
	}
	select {
	case p.jobs <- t:
		return nil
	default: // 队列已满，进入扩容分支
	}
	p.mu.Lock()
	if p.active.Load() < p.max {
		p.scale(1)
	}
	p.mu.Unlock()

	select {
	case p.jobs <- t:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

// Close 关闭队列，等待所有 worker 退出，并汇总全部错误。
func (p *Pool) Close() error {
	p.closed.Store(true)
	close(p.jobs)
	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.Join(p.errs...)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewPool(ctx, 2, 6, 4) // 最少 2 个 worker，最多 6 个，任务队列容量 4

	var ok, failed atomic.Int32
	for i := 1; i <= 24; i++ {
		id := i
		err := pool.Submit(func(ctx context.Context) error {
			select {
			case <-time.After(20 * time.Millisecond): // 模拟耗时
			case <-ctx.Done():
				return ctx.Err()
			}
			if id%7 == 0 {
				failed.Add(1)
				return fmt.Errorf("处理任务 %d: %w", id, errTaskFailed)
			}
			ok.Add(1)
			return nil
		})
		if err != nil {
			fmt.Println("提交失败:", err)
		}
	}

	err := pool.Close()
	fmt.Println("成功:", ok.Load(), "失败:", failed.Load())
	fmt.Println("汇总错误:", err)
	fmt.Println("首个错误可用 errors.Is 判定:", errors.Is(err, errTaskFailed))
	fmt.Println("结束时存活 worker:", pool.active.Load())
}
```

运行结果：

```text
成功: 21 失败: 3
汇总错误: 处理任务 7: 任务执行失败
处理任务 14: 任务执行失败
处理任务 21: 任务执行失败
首个错误可用 errors.Is 判定: true
结束时存活 worker: 0
```

解读：

- **扩容时机**：`Submit` 先用非阻塞 `select` 尝试入队，队列满时才考虑加人（并且受 `mu` + 双重检查保护，避免并发提交把 worker 数冲过 `max`）。这是"按需扩容"的标准写法。
- **缩容必须加两道护栏**：一是"空闲超时"（这里是 60 毫秒没有活干），二是"没有积压任务且总数高于下限"。少了任何一条，都可能在任务洪峰刚过去时把 worker 全撤掉，导致后续任务无人处理。
- **`idleTimer` 的 `Stop` + 排空 + `Reset`** 是 Go 定时器的标准三连。如果只 `Reset` 不排空，旧的那个已触发的信号会留在 channel 里，导致下一次 `select` 立刻误判为空闲超时——这是一个非常隐蔽的 bug。
- **错误收集**用 mutex + 切片 + `errors.Join`（见 11.5.1），"只记第一个"则用 `sync.Once`。
- **`Close` 的顺序是"先关队列、再 `Wait`"**：关闭 `jobs` 让所有 worker 在排空后自然返回，`wg.Wait()` 才可能返回；顺序反了就是死锁。
- 最后 `active=0` 是预期的：`Close` 之后所有 worker 都会退出，池子不该再被复用。

💡 优缺点：优点是并发度可控、资源可预算、支持优雅关闭；缺点是实现复杂度高（定时器、扩容竞态、关闭语义都要处理），任务极短（亚微秒）时 channel 的调度开销会超过任务本身的耗时。

### 11.4.5 Semaphore：限流而不重建协程

**适用场景**：不想维护一个池子，只想给某段代码"限个并发数"。典型如"最多同时抓 10 个网页"、"最多 4 个图片转码任务"。

下面这份完整程序同时演示三件事：手写信号量（`fetchSimple`）、`golang.org/x/sync/semaphore` 的加权版本（`processImages`，总权重上限 4，小图可以 4 张一起跑、大图一次只能 1 张），以及非阻塞的 `TryAcquire`。运行前需要先 `go get golang.org/x/sync`。

```go
// 摘自本章验证用例 d05_semaphore/main.go
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

// fetchSimple 用手写信号量（容量为 limit 的带缓冲 channel）限制并发。
func fetchSimple(limit int, urls []string) int32 {
	sem := make(chan struct{}, limit) // 令牌桶：缓冲区里的每个元素就是一个令牌
	var wg sync.WaitGroup
	var done atomic.Int32

	for _, u := range urls {
		sem <- struct{}{} // 获取令牌，满了就阻塞
		wg.Go(func() {
			defer func() { <-sem }() // 释放令牌，必须 defer 以防 panic 泄漏
			time.Sleep(20 * time.Millisecond)
			done.Add(1)
			fmt.Println("完成:", u)
		})
	}
	wg.Wait()
	return done.Load()
}

// image 用一个权重代表它占用的资源，大图占得多。
type image struct {
	name   string
	weight int64
	bytes  int64
}

// processImages 用 x/sync/semaphore 做加权并发控制：
// 总权重上限 4，小图可以 4 张一起跑，大图一次只能 1 张。
func processImages(ctx context.Context, imgs []image) (int64, error) {
	sem := semaphore.NewWeighted(4)
	var total atomic.Int64

	var wg sync.WaitGroup
	errs := make([]error, len(imgs))

	for i, img := range imgs {
		if err := sem.Acquire(ctx, img.weight); err != nil {
			return total.Load(), fmt.Errorf("获取信号量失败: %w", err)
		}
		wg.Go(func() {
			defer sem.Release(img.weight)
			select {
			case <-time.After(20 * time.Millisecond):
				total.Add(img.bytes)
			case <-ctx.Done():
				errs[i] = ctx.Err()
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return total.Load(), err
		}
	}
	return total.Load(), nil
}

func main() {
	urls := make([]string, 0, 8)
	for i := 1; i <= 8; i++ {
		urls = append(urls, fmt.Sprintf("https://example.com/%d", i))
	}
	fmt.Println("手写信号量（并发上限 2）完成:", fetchSimple(2, urls), "个")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	imgs := []image{
		{"a.jpg", 1, 100},
		{"big.png", 4, 400},
		{"b.jpg", 1, 200},
		{"c.jpg", 2, 300},
	}
	total, err := processImages(ctx, imgs)
	fmt.Println("加权信号量处理字节数:", total, "错误:", err)

	// 权重超限时的行为：TryAcquire 立刻返回 false，不会阻塞
	sem := semaphore.NewWeighted(2)
	fmt.Println("TryAcquire(2):", sem.TryAcquire(2))
	fmt.Println("TryAcquire(1):", sem.TryAcquire(1))
	sem.Release(2)
	fmt.Println("Release 后可再次获取:", sem.TryAcquire(2))
}
```

运行结果：

```text
手写信号量（并发上限 2）完成: 8 个
加权信号量处理字节数: 1000 错误: <nil>
TryAcquire(2): true
TryAcquire(1): false
Release 后可再次获取: true
```

解读：

- **带缓冲 channel 就是计数信号量**：缓冲区里放一个元素代表"占用一个名额"，`len(sem)` 就是当前占用数。这个技巧简单到不需要任何库。
- **释放必须写成 `defer func() { <-sem }()`**。注意 `defer <-sem` 根本不是合法语法（`defer` 后面只能跟函数调用），所以闭包是唯一写法。更要紧的是**必须 `defer`**：任务 panic 时若没有释放，这个名额就永久丢了，池子会慢慢"漏水"直到完全不工作。
- **手写版本的两个短板**：一是无法按权重获取（一张大图占 4 个名额）；二是**无法感知 ctx 取消**——`sem <- struct{}{}` 阻塞时收到取消也没法退出。需要这两点时换 `x/sync/semaphore`。
- **加权信号量的语义**：`Acquire(ctx, n)` 是"要么拿到 n 个名额，要么等"，不会拿到一半。因此**权重不能超过总容量**，否则会永久阻塞（这一点务必在配置里校验）。
- **`TryAcquire` 是非阻塞版**：拿不到立刻返回 `false`，适合"宁可降级也不排队"的场景。

💡 优缺点：优点是实现极简、开销小、思路清晰；缺点是不具备任务队列（提交方会阻塞）、无法优先级调度，也不像 worker pool 那样能回收闲置资源。

### 11.4.6 Rate Limiter：从手写令牌桶到 x/time/rate

**适用场景**：保护下游（第三方 API 有 QPS 限制）、保护自己（限制单用户请求速率）、平滑流量毛刺。

先看手写版本，理解令牌桶的本质。这份完整程序前半段是手写的 `tokenBucket`，后半段则用 `golang.org/x/time/rate` 演示工业级实现（运行前需要 `go get golang.org/x/time`）：

```go
// 摘自本章验证用例 d06_ratelimit/main.go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// tokenBucket 是手写的令牌桶：容量固定，按固定速率补充，取不到就拒绝。
type tokenBucket struct {
	mu       sync.Mutex
	capacity float64
	tokens   float64
	rate     float64 // 每秒补充的令牌数
	last     time.Time
}

func newTokenBucket(ratePerSec, capacity float64) *tokenBucket {
	return &tokenBucket{
		capacity: capacity,
		tokens:   capacity, // 初始装满，允许一次突发
		rate:     ratePerSec,
		last:     time.Now(),
	}
}

// allow 尝试取走 1 个令牌，取不到立即返回 false（非阻塞）。
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.tokens = min(b.capacity, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func main() {
	// 1) 手写令牌桶：每秒 10 个，桶容量 5，瞬间打 20 个请求
	b := newTokenBucket(10, 5)
	allowed := 0
	for i := 0; i < 20; i++ {
		if b.allow() {
			allowed++
		}
	}
	fmt.Println("瞬时 20 个请求通过:", allowed, "（等于桶容量 5，其余被拒）")

	time.Sleep(200 * time.Millisecond) // 200ms 补充约 2 个令牌
	allowed = 0
	for i := 0; i < 5; i++ {
		if b.allow() {
			allowed++
		}
	}
	fmt.Println("等待 200ms 后通过:", allowed, "（约等于补充的令牌数）")

	// 2) x/time/rate：Allow 非阻塞，Wait 阻塞并支持 ctx 取消
	lim := rate.NewLimiter(rate.Limit(20), 3) // 20 次/秒，突发上限 3
	fmt.Println("Allow 立即判断:", lim.Allow(), lim.Allow(), lim.Allow(), lim.Allow())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	passed := 0
	for i := 0; i < 6; i++ {
		if err := lim.Wait(ctx); err != nil {
			fmt.Println("等待令牌被取消:", err)
			break
		}
		passed++
	}
	fmt.Printf("Wait 方式通过 %d 次，耗时 %v\n", passed, time.Since(start).Round(10*time.Millisecond))

	// 3) 用 Reserve 拿到"需要等多久"的信息，自己做调度决策
	r := lim.Reserve()
	fmt.Println("下一次可用需等待:", r.Delay() > 0)
	r.Cancel() // 不打算用了必须 Cancel，否则会白白占掉一个令牌
}
```

运行结果：

```text
瞬时 20 个请求通过: 5 （等于桶容量 5，其余被拒）
等待 200ms 后通过: 2 （约等于补充的令牌数）
Allow 立即判断: true true true false
Wait 方式通过 6 次，耗时 300ms
下一次可用需等待: true
```

解读：

- **令牌桶的两个参数决定行为**：`rate`（补充速度，决定长期平均速率）与 `burst`（桶容量，决定能容忍多大的瞬时突发）。上面 `rate=20/s, burst=3` 时，前 3 个请求瞬时通过，之后每个要等 50 毫秒，所以 6 个请求总共约 300 毫秒（前 3 个消耗了最初的突发额度）。
- **`Allow` / `Wait` / `Reserve` 三选一**，取决于你要"拒绝"还是"排队"：`Allow` 立刻拒绝，适合"过载就降级"；`Wait` 排队等待且可被 ctx 取消，适合"宁可慢也不能丢"；`Reserve` 返回一个预约对象，让你自己决定等不等，`Delay()` 给出等待时长。
- **`Reserve` 之后必须 `Cancel()`**：预约会立刻从桶里扣掉令牌，不打算用了却不取消，就等于每天白白吞掉配额。这是该库最容易踩的坑。
- **不要每个请求都 `NewLimiter`**：limiter 必须作为长生命周期的对象共享（比如挂在客户端结构体上），否则等于没有限流。
- **分布式场景下它们都不够用**：多个实例各自限流等于把总量放大了 N 倍，此时需要 Redis 令牌桶之类的集中式方案。

💡 优缺点：优点是实现小、语义清晰、突发容忍可控；缺点是单机版无法跨实例协调，且 `Wait` 在排队时会短暂占用 goroutine。

### 11.4.7 Or-Done / Tee / Bridge：channel 组合子

这三个来自 Rob Pike 著名的演讲 "Go Concurrency Patterns"，是构建更复杂数据流的基本零件（示例使用泛型，因此需要 Go 1.18+）。

```go
// 摘自本章验证用例 d07_combine/main.go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// orDone 是"取消感知"的包装器：无论上游 channel 是否关闭，
// ctx 一取消，输出 channel 必定被关闭，调用方绝不会被卡死。
func orDone[T any](ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// tee 把一个流复制成两份，两份都会收到全部元素。
func tee[T any](ctx context.Context, in <-chan T) (<-chan T, <-chan T) {
	out1 := make(chan T)
	out2 := make(chan T)
	go func() {
		defer close(out1)
		defer close(out2)
		for v := range orDone(ctx, in) {
			o1, o2 := out1, out2
			for range 2 { // 必须成功投递到两路，缺一不可
				select {
				case o1 <- v:
					o1 = nil // 置 nil 后这一路永久阻塞，select 不会再选中它
				case o2 <- v:
					o2 = nil
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out1, out2
}

// bridge 把 channel 的 channel 展平成一个流，用于"分批查询、顺序消费"。
func bridge[T any](ctx context.Context, chans <-chan <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ch, ok := <-chans:
				if !ok {
					return
				}
				for v := range orDone(ctx, ch) {
					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out
}

func gen(ctx context.Context, nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			select {
			case out <- n:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// pages 模拟分页接口：每一页是一个独立的 channel
func pages(ctx context.Context, per int, total int) <-chan <-chan int {
	out := make(chan (<-chan int))
	go func() {
		defer close(out)
		for start := 1; start <= total; start += per {
			nums := make([]int, 0, per)
			for i := start; i < start+per && i <= total; i++ {
				nums = append(nums, i)
			}
			select {
			case out <- gen(ctx, nums...):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1) tee：同一份数据喂给两个消费者。
	//    注意两路必须"并发消费"：tee 内部要等两路都投递成功才会处理下一个元素，
	//    先读干 a 再读 b 会直接死锁。
	a, b := tee(ctx, gen(ctx, 1, 2, 3, 4, 5))
	sums := make([]int, 2)
	var wg sync.WaitGroup
	for i, ch := range []<-chan int{a, b} {
		wg.Go(func() {
			for v := range ch {
				sums[i] += v
			}
		})
	}
	wg.Wait()
	fmt.Println("tee 两路结果一致:", sums[0], sums[1])

	// 2) bridge：展平分页结果
	sum := 0
	count := 0
	for v := range bridge(ctx, pages(ctx, 3, 10)) {
		sum += v
		count++
	}
	fmt.Println("bridge 展平:", count, "个元素，和 =", sum)

	// 3) orDone：上游不关闭时，取消 ctx 也能让下游安全退出
	stuck := make(chan int) // 永远不会有数据的"僵尸" channel
	done := make(chan struct{})
	go func() {
		for range orDone(ctx, stuck) {
		}
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
		fmt.Println("orDone 及时关闭，下游未被卡死")
	case <-time.After(time.Second):
		fmt.Println("不应该走到这里")
	}
}
```

运行结果：

```text
tee 两路结果一致: 15 15
bridge 展平: 10 个元素，和 = 55
orDone 及时关闭，下游未被卡死
```

解读：

- **`orDone` 是所有组合子的安全底座**：它保证"只要 ctx 取消，输出 channel 一定会被关闭"，于是下游的 `for range` 永远能退出。任何"可能永远不关闭的上游"（比如别人写的不规范库）都应该先过一遍 `orDone`。第三个输出场景专门验证了这点：一个永不发送、也永不关闭的 `stuck` channel，被 `orDone` 包装后，`cancel()` 一调用下游就优雅退出了。
- **`tee` 的 `o1 = nil` 技巧值得细品**：往一个 `nil` channel 发送会永久阻塞，所以把已投递完成的那一路置为 `nil` 之后，`select` 就再也不会选中它，循环里自然只剩"另一路 + 取消"。同一份数据要发给两个消费者时，这个模式比手工写两个 channel 更不容易漏。
- **`tee` 的两路必须并发消费**。因为 tee 是"先投递完这一路、再投递下一路"，如果上游一直发、而消费者只读其中一路，tee 会在第二路上阻塞，进而把整条流水线堵死。本示例用两个 goroutine 分别读取，才不会死锁——这是 tee 最容易踩的坑。
- **`bridge` 解决的是"paginated API"问题**：分页接口天然返回"批次的批次"，桥接之后调用方可以像读一个普通流一样 `for range`，不用关心分页。

💡 优缺点：优点是这些小零件组合性极强、泛型化后可复用；缺点是每个组合子都引入一个 goroutine 与一次拷贝，且 `tee` 的语义（阻塞式双路投递）要求消费者配合，滥用会导致隐蔽死锁。

### 11.4.8 Future / Promise：把异步结果封装成对象

**适用场景**：多个互相独立的耗时操作可以并发发起，但需要在一个统一的地方取结果；或者希望把"发起"与"取结果"在时间上分开（例如先发 3 个请求，再依次处理响应）。

```go
// 摘自本章验证用例 d08_future/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Future 表示一个将来才会有结果的异步计算。
type Future[T any] struct {
	done chan struct{} // 关闭即代表结果就绪
	val  T
	err  error
}

// Async 立刻返回，计算在后台 goroutine 中进行。
func Async[T any](ctx context.Context, f func(ctx context.Context) (T, error)) *Future[T] {
	fut := &Future[T]{done: make(chan struct{})}
	go func() {
		defer close(fut.done) // close 之前的赋值对 Get 一方必然可见
		fut.val, fut.err = f(ctx)
	}()
	return fut
}

// Get 等待结果；ctx 到期则放弃等待，但后台计算仍会继续（除非它自己监听 ctx）。
func (f *Future[T]) Get(ctx context.Context) (T, error) {
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// Ready 非阻塞地看一眼结果是否就绪。
func (f *Future[T]) Ready() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

func fetchPrice(ctx context.Context, sku string, cost time.Duration, fail bool) (float64, error) {
	select {
	case <-time.After(cost):
		if fail {
			return 0, fmt.Errorf("查询 %s: %w", sku, errUpstream)
		}
		return 99.5, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

var errUpstream = errors.New("上游服务不可用")

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// 三个耗时操作并发启动，总耗时约等于最慢的那个
	start := time.Now()
	fA := Async(ctx, func(ctx context.Context) (float64, error) {
		return fetchPrice(ctx, "A", 80*time.Millisecond, false)
	})
	fB := Async(ctx, func(ctx context.Context) (float64, error) {
		return fetchPrice(ctx, "B", 60*time.Millisecond, true)
	})
	fC := Async(ctx, func(ctx context.Context) (float64, error) {
		return fetchPrice(ctx, "C", 120*time.Millisecond, false)
	})

	fmt.Println("B 是否已就绪（多半为 false）:", fB.Ready())

	pA, errA := fA.Get(ctx)
	pB, errB := fB.Get(ctx)
	pC, errC := fC.Get(ctx)
	fmt.Printf("A=%.1f C=%.1f 耗时=%v\n", pA, pC, time.Since(start).Round(10*time.Millisecond))
	fmt.Println("B 的报价(失败时为 0):", pB)
	fmt.Println("B 的错误:", errB, "可用 errors.Is 判定:", errors.Is(errB, errUpstream))
	_ = errA
	_ = errC
	fmt.Println("B 是否已就绪（此时必为 true）:", fB.Ready())

	// 慢任务 + 短超时：Get 提前放弃
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	slow := Async(ctx2, func(ctx context.Context) (string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "太慢了", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	if _, err := slow.Get(ctx2); err != nil {
		fmt.Println("提前放弃等待:", err)
	}
}
```

运行结果：

```text
B 是否已就绪（多半为 false）: false
A=99.5 C=99.5 耗时=120ms
B 的报价(失败时为 0): 0
B 的错误: 查询 B: 上游服务不可用 可用 errors.Is 判定: true
B 是否已就绪（此时必为 true）: true
提前放弃等待: context deadline exceeded
```

解读：

- **`val`/`err` 的写入安全吗？** 安全，这正是 happens-before 的应用：写 `val`/`err` 发生在 `close(fut.done)` **之前**，而 `Get` 读到 `done` 关闭发生在读 `val`/`err` **之前**，于是"close 先于因关闭而收到零值"这条规则把可见性建立起来了。如果没有 `close` 这一手，就必须加锁或改用 `atomic.Pointer`。
- **`Future` 只允许取一次结果吗？** 不，`done` 关闭后可以反复 `Get`，每次都会立即返回同一个结果。这让它天然支持"多个消费者共享一个异步结果"。
- **"放弃等待"与"取消计算"是两件事**。`Get` 因为 ctx 超时返回时，后台 goroutine 仍然在跑——除非传给 `Async` 的 ctx 也被取消。真实代码里应当把同一个 ctx 传进去，让两者一致。
- **适合它的是"扇出后收集"**：三个查询并发发出，总耗时约等于最慢的那个（示例里 120 毫秒），而不是三者之和。

💡 优缺点：优点是调用方代码看起来是同步的（`f.Get(ctx)`），心智负担低；缺点是泛型版的 Future 每次都会起一个 goroutine（哪怕结果已经算好），且比朴素的 `errgroup` 多一层封装。**在 Go 里，多数"Future"场景用 `errgroup` 或带缓冲的 channel 就够了**，Future 主要的价值是让"先发起、后取值"的时序显式化。

### 11.4.9 ErrGroup：并发任务 + 错误传播的标准答案

需要 `golang.org/x/sync/errgroup`。它是"一组并发的子任务 + 只关心第一个错误"这件事的官方实现。

```go
// 摘自本章验证用例 d09_errgroup/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

var errBoom = errors.New("任务内部故障")

func main() {
	// 1) 基础用法：Wait 只返回"第一个"非 nil 错误
	var g errgroup.Group
	for i := 1; i <= 3; i++ {
		id := i
		g.Go(func() error {
			if id == 2 {
				return errBoom
			}
			return nil
		})
	}
	fmt.Println("基础用法 Wait 返回:", g.Wait())

	// 2) WithContext + SetLimit：任一任务失败会取消 ctx，但不会杀死兄弟 goroutine，
	//    兄弟必须自己监听 ctx.Done() 才会提前退出。
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	eg, ctx := errgroup.WithContext(parent)
	eg.SetLimit(2) // 同时最多 2 个 goroutine，超出的 Go 调用会阻塞等待空位

	for i := 1; i <= 6; i++ {
		id := i
		eg.Go(func() error {
			select {
			case <-time.After(30 * time.Millisecond):
			case <-ctx.Done():
				fmt.Println("任务", id, "看到取消信号:", ctx.Err())
				return ctx.Err()
			}
			if id == 4 {
				return fmt.Errorf("任务 %d: %w", id, errBoom)
			}
			fmt.Println("任务", id, "成功")
			return nil
		})
	}
	err := eg.Wait()
	fmt.Println("Wait 返回值:", err)
	fmt.Println("errors.Is 判定:", errors.Is(err, errBoom))

	// 3) 用 SetLimit 之外的另一种限流：配合 semaphore 可以实现"按权重限流"，
	//    这里演示 errgroup 只负责"收集第一个错误"这一件事。
}
```

运行结果：

```text
基础用法 Wait 返回: 任务内部故障
任务 1 成功
任务 2 成功
任务 3 成功
任务 6 看到取消信号: context canceled
任务 5 看到取消信号: context canceled
Wait 返回值: 任务 4: 任务内部故障
errors.Is 判定: true
```

解读：

- **`Wait` 只返回第一个非 nil 错误**，其余错误被丢弃。想知道全部失败原因，就回到 11.5.1 的 `errors.Join` 或错误 channel 方案。
- **`WithContext` 返回的 ctx 会在"第一个错误发生"或"Wait 返回"时被取消**。它的作用是让还在跑的任务尽快收手，但**它不会杀死任何 goroutine**——上面输出里任务 5、6 之所以能提前退出，是因为它们在 `select` 里监听了 `ctx.Done()`。如果任务体是个纯 `time.Sleep`，它照样会睡完。
- **`SetLimit(2)` 控制并发数**：超出的 `Go` 调用会阻塞，直到有空位。这让 errgroup 同时具备 worker pool 的能力，且不需要自己写队列。注意 `Go` 变成阻塞调用之后，"提交任务"这个动作本身也会被限流，循环里要能接受这种背压。
- **不要忽略 `errgroup.Group`（不带 ctx 的零值用法）**：`var g errgroup.Group` 就能直接用，适合不需要联动取消的简单场景。
- **两个容易踩的坑**：一是循环变量捕获（Go 1.22 之后 `for` 循环变量每次迭代独立，可以放心直接用，但**用 `for i := 0; i < n; i++` + 闭包时仍建议 `id := i` 明示**，这份示例保留了这一习惯以避免歧义）；二是 **`Wait` 必须被调用**，它会在返回前取消那个派生出来的 Context；不调用 `Wait`，派生出的子 Context 就会一直挂在父节点上（与 11.1.4 讨论的泄漏同源），调用方也永远拿不到结果。
- **顺带一个不那么知名但很实用的细节**：`errgroup.WithContext` 内部用的是 `context.WithCancelCause`，而"第一个错误"被当作 cause 传了进去。因此在子任务里可以直接用 `context.Cause(ctx)` 拿到那个原始错误，不必自己额外传递。

💡 优缺点：优点是 API 极小、与 ctx 无缝集成、错误语义清晰，是绝大多数"并发跑 N 个任务"场景的默认选择；缺点是只保留第一个错误、没有任务优先级与队列，且 `SetLimit` 的阻塞式提交需要调用方理解背压。

### 11.4.10 Singleflight：合并重复请求

需要 `golang.org/x/sync/singleflight`。它解决的是**缓存击穿**：某个热点 key 过期的一瞬间，成千上万个请求同时回源查数据库。

```go
// 摘自本章验证用例 d10_singleflight/main.go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

var (
	sf      singleflight.Group
	dbCalls atomic.Int32

	mu    sync.Mutex
	cache = map[string]string{}
)

// queryFromDB 模拟一次昂贵且缓慢的回源查询
func queryFromDB(key string) (string, error) {
	dbCalls.Add(1)
	time.Sleep(50 * time.Millisecond)
	return "value-of-" + key, nil
}

// get 是"缓存 + singleflight"的标准写法：
// 缓存判定必须放在 singleflight 的函数内部做二次检查，
// 否则先穿过缓存检查、后进入 Do 的请求会再打一次数据库。
func get(key string) (string, error) {
	v, err, shared := sf.Do(key, func() (any, error) {
		mu.Lock()
		if val, ok := cache[key]; ok {
			mu.Unlock()
			return val, nil
		}
		mu.Unlock()

		val, err := queryFromDB(key)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		cache[key] = val
		mu.Unlock()
		return val, nil
	})
	if err != nil {
		return "", err
	}
	_ = shared // shared 为 true 表示结果来自其他 goroutine 的调用
	return v.(string), nil
}

func main() {
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if _, err := get("user:1001"); err != nil {
				fmt.Println("查询失败:", err)
			}
		})
	}
	wg.Wait()
	fmt.Println("100 个并发请求，实际回源次数:", dbCalls.Load())

	if v, err := get("user:1001"); err == nil {
		fmt.Println("再次访问命中缓存:", v)
	}
	fmt.Println("总回源次数:", dbCalls.Load())

	// Forget 用于"结果已知失效"的场景：调用后不再复用正在进行的结果
	sf.Forget("user:1001")
	mu.Lock()
	delete(cache, "user:1001")
	mu.Unlock()
	if _, err := get("user:1001"); err == nil {
		fmt.Println("Forget 并清缓存后，回源次数:", dbCalls.Load())
	}
}
```

运行结果：

```text
100 个并发请求，实际回源次数: 1
再次访问命中缓存: value-of-user:1001
总回源次数: 1
Forget 并清缓存后，回源次数: 2
```

解读：

- **为什么缓存判定要写在 `Do` 内部？** 如果像常见写法那样先在外部查缓存、未命中再调 `Do`，那么"已经穿过缓存检查、但还没进入 `Do`"的请求会在第一个 `Do` 结束后再次触发回源。把二次检查放进 `Do` 的函数体里，这类请求就会命中缓存直接返回，回源次数因此稳定在 1（示例中 100 个并发请求只回源 1 次，正是这个道理）。
- **回源函数的返回值类型是 `any`，需要断言**。这是 `singleflight` 缺泛型的历史包袱，写的时候记得 `v.(string)` 之前确认调用方传的就是这个类型；对同一个 key 混用不同类型会 panic。
- **第三个返回值 `shared`** 表示"这个结果是从别人的调用里拿到的"。可以用它做统计（合并率），也可以据此决定是否要二次深拷贝（因为多个调用方拿到的是**同一个**值，若它是可变对象，修改会互相影响）。
- **`Do` 会共享错误**：第一个调用的错误会被所有等待者收到。
- **`Forget(key)`** 用于"结果已确定失效"的场景（例如收到一条更新消息），调用后正在进行中的结果不再被复用；**`DoChan`** 则返回一个 channel，适合需要 `select` 超时控制的场景。

⚠️ 注意：`singleflight` 只合并**同一进程内**的并发请求。多实例部署时，仍然需要分布式锁或"逻辑过期 + 异步刷新"来防止整体回源。

### 11.4.11 Pub/Sub：一对多广播

**适用场景**：一次事件需要通知多个互不相关的处理者（订单创建后：发短信、记积分、推送到风控）。

```go
// 摘自本章验证用例 d11_pubsub/main.go
package main

import (
	"fmt"
	"sync"
)

// PubSub 是最简发布订阅：主题 -> 订阅者集合。
type PubSub[T any] struct {
	mu   sync.RWMutex
	subs map[string]map[chan T]struct{}
	buf  int
}

func NewPubSub[T any](buf int) *PubSub[T] {
	return &PubSub[T]{subs: make(map[string]map[chan T]struct{}), buf: buf}
}

// Subscribe 返回专属 channel 和退订函数。退订函数用 sync.Once 保证幂等。
func (p *PubSub[T]) Subscribe(topic string) (<-chan T, func()) {
	ch := make(chan T, p.buf)

	p.mu.Lock()
	if p.subs[topic] == nil {
		p.subs[topic] = make(map[chan T]struct{})
	}
	p.subs[topic][ch] = struct{}{}
	p.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			p.mu.Lock()
			if set, ok := p.subs[topic]; ok {
				delete(set, ch)
				if len(set) == 0 {
					delete(p.subs, topic) // 回收空主题，避免 map 无限增长
				}
			}
			p.mu.Unlock()
			// 此刻 ch 已从集合里删除：Publish 全程持读锁、退订持写锁，
			// 两者互斥，因此 close 不可能与任何正在进行的发送并发
			close(ch)
		})
	}
	return ch, unsubscribe
}

// Publish 广播给所有订阅者。慢订阅者丢消息，绝不阻塞发布方。
func (p *PubSub[T]) Publish(topic string, msg T) (delivered, dropped int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for ch := range p.subs[topic] {
		select {
		case ch <- msg:
			delivered++
		default:
			dropped++
		}
	}
	return delivered, dropped
}

func (p *PubSub[T]) Subscribers(topic string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subs[topic])
}

func main() {
	ps := NewPubSub[string](2) // 每个订阅者 2 格缓冲

	chA, unsubA := ps.Subscribe("order.created")
	chB, unsubB := ps.Subscribe("order.created")
	fmt.Println("订阅者数量:", ps.Subscribers("order.created"))

	// 发布 3 条消息，但先不消费：2 格缓冲装满后第 3 条被丢弃
	for i := 1; i <= 3; i++ {
		d, drop := ps.Publish("order.created", fmt.Sprintf("order-%d", i))
		fmt.Printf("发布 order-%d: 投递 %d, 丢弃 %d\n", i, d, drop)
	}

	// 非阻塞地把缓冲区里已有的消息取空（channel 尚未关闭，不能用 for range）
	read := func(name string, ch <-chan string, n int) {
		for i := 0; i < n; i++ {
			select {
			case v := <-ch:
				fmt.Println(name, "收到:", v)
			default:
				return
			}
		}
	}
	read("A", chA, 2)
	read("B", chB, 2)

	unsubA()
	unsubB()
	unsubA() // 幂等：重复退订不会 panic
	fmt.Println("退订后订阅者数量:", ps.Subscribers("order.created"))

	// 退订后 channel 已关闭，读取会立刻得到零值与 ok=false
	if v, ok := <-chA; !ok {
		fmt.Println("chA 已关闭，读到零值:", v == "")
	}
	d, drop := ps.Publish("nobody.listens", "hi")
	fmt.Println("向无订阅者的主题发布也安全:", d, drop)
}
```

运行结果：

```text
订阅者数量: 2
发布 order-1: 投递 2, 丢弃 0
发布 order-2: 投递 2, 丢弃 0
发布 order-3: 投递 0, 丢弃 2
A 收到: order-1
A 收到: order-2
B 收到: order-1
B 收到: order-2
退订后订阅者数量: 0
chA 已关闭，读到零值: true
向无订阅者的主题发布也安全: 0 0
```

解读：

- **"每个订阅者一个 channel"是这个模式的核心**：`map[string]map[chan T]struct{}` 用集合而不是切片，退订是 O(1) 的 `delete`，也不需要遍历找位置。
- **`Publish` 用非阻塞发送**：订阅者消费慢了就丢弃（示例里第 3 条消息给两路都丢了，因为每个订阅者只有 2 格缓冲）。这是一个**明确的取舍**：要么丢消息保证发布方不被拖死，要么阻塞等慢订阅者。真实系统里通常选前者，并把 `dropped` 上报成监控指标。
- **`close(ch)` 为什么安全？** 关键不在"`close` 那一刻是否还持有写锁"（代码里 `close(ch)` 写在 `p.mu.Unlock()` 之后），而在于**这次退订已经把 `ch` 从订阅者集合里删掉了**：`Publish` 全程持有读锁，退订持有写锁，两者互斥。于是只有两种可能——要么 `Publish` 的读锁临界区（含所有发送）先全部结束，退订才能拿到写锁；要么退订先完成，之后新来的 `Publish` 在集合里已经找不到 `ch`、根本不会去发送。`close` 因此不可能与任何发送并发。如果这里图省事用了 `RLock`，两个"读锁持有者"可以并行，就会得到"send on closed channel"的 panic。
- **退订函数用 `sync.Once` 包装**：重复退订、并发退订都安全（示例里故意调用了两次）。
- **空主题要回收**：`len(set) == 0` 时删除整个 key，否则长期运行的服务会在 map 里累积大量空主题。

💡 优缺点：优点是实现不到 60 行、语义直观、支持动态订阅；缺点是**没有持久化与重投**（消息丢了就没了）、没有顺序保证之外的投递保证，也不适合跨进程。生产环境一般直接上 NATS、Redis Streams 或 Kafka。

### 11.4.12 超时、重试与熔断

**适用场景**：调用不可靠的下游。重试解决偶发抖动，熔断解决"下游已经挂了，再怎么重试都是白费"。

先看带指数退避与抖动的重试器：

```go
// 摘自本章验证用例 d12_retry/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

var (
	errTransient = errors.New("临时故障，可以重试")
	errPermanent = errors.New("参数非法，重试无意义")
)

// Retry 带指数退避与抖动的重试器：
//   - attempts 为最大尝试次数；
//   - base 为首次退避时长，之后每次翻倍；
//   - 听到 ctx 取消立刻返回，不会白等；
//   - 不可重试的错误立即上抛。
func Retry(ctx context.Context, attempts int, base time.Duration,
	fn func(ctx context.Context) error) error {

	if attempts < 1 {
		attempts = 1
	}
	var last error
	delay := base

	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("重试被中断: %w", err)
		}
		last = fn(ctx)
		if last == nil {
			return nil
		}
		if errors.Is(last, errPermanent) {
			return fmt.Errorf("不可重试: %w", last)
		}
		if i == attempts {
			break
		}

		jitter := 0.5 + rand.Float64() // 0.5 ~ 1.5 倍抖动，避免惊群
		wait := time.Duration(float64(delay) * jitter)
		fmt.Printf("第 %d 次失败(%v)，%v 后重试\n", i, last, wait.Round(time.Millisecond))

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("等待重试时被取消: %w", ctx.Err())
		}
		delay *= 2
	}
	return fmt.Errorf("重试 %d 次后仍失败: %w", attempts, last)
}

func main() {
	// 场景一：前两次失败，第三次成功
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	calls := 0
	err := Retry(ctx, 5, 20*time.Millisecond, func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return fmt.Errorf("第 %d 次调用: %w", calls, errTransient)
		}
		return nil
	})
	fmt.Println("场景一结果:", err, "调用次数:", calls)

	// 场景二：不可重试的错误，立即返回
	err = Retry(ctx, 5, 20*time.Millisecond, func(ctx context.Context) error {
		return fmt.Errorf("请求参数 id=0: %w", errPermanent)
	})
	fmt.Println("场景二结果:", err, "被判定为不可重试:", errors.Is(err, errPermanent))

	// 场景三：上游一直失败，ctx 提前到期
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer shortCancel()
	attempts := 0
	err = Retry(shortCtx, 10, 100*time.Millisecond, func(ctx context.Context) error {
		attempts++
		return errTransient
	})
	fmt.Println("场景三结果:", err)
	fmt.Println("在超时预算内只尝试了", attempts, "次")
}
```

运行结果（退避时长含随机抖动，因此每次略有不同）：

```text
第 1 次失败(第 1 次调用: 临时故障，可以重试)，29ms 后重试
第 2 次失败(第 2 次调用: 临时故障，可以重试)，55ms 后重试
场景一结果: <nil> 调用次数: 3
场景二结果: 不可重试: 请求参数 id=0: 参数非法，重试无意义 被判定为不可重试: true
第 1 次失败(临时故障，可以重试)，81ms 后重试
第 2 次失败(临时故障，可以重试)，236ms 后重试
场景三结果: 等待重试时被取消: context deadline exceeded
在超时预算内只尝试了 2 次
```

解读：

- **区分可重试与不可重试的错误是重试器的第一要务**。参数非法、鉴权失败、404 这类错误重试一万次也没用，只会放大故障；网络抖动、超时、限流（429）才值得重试。示例用 `errors.Is(last, errPermanent)` 做判定，实际项目里可以再细化成"状态码/错误类型 → 策略"的映射表。
- **指数退避 + 抖动**是标准组合。不加抖动时，所有客户端会在同一时刻重试，形成周期性的流量尖峰（惊群 / thundering herd）；抖动把重试点打散，代价只是一点平均延迟。
- **重试必须受 ctx 约束**。第三段输出说明：150 毫秒的总预算里只来得及尝试 2 次，程序没有"为了重试而突破超时预算"。**重试预算是超时预算的一部分**，不是额外的。
- **重试要放在合适的层级**。在每一层都加重试会指数级放大调用次数（3 层各重试 3 次 = 27 倍流量），通常只在最外层（客户端边界）重试一次。
- **重试要考虑幂等性**。非幂等的写操作（扣款、下单）必须有幂等键，否则重试就是重复下单。

再看熔断器。它的思路是一个状态机：

```text
         失败数达到阈值                      冷却时间到，
   +---------------------->  OPEN  -------------------------+
   |                        (直接拒绝)                        v
 CLOSED  <--------------  HALF-OPEN  <---- 只放一个探测请求
   ^        探测成功            |
   |                            | 探测失败
   +----------------------------+--> 重新 OPEN
```

```go
// 摘自本章验证用例 d14_breaker/main.go
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var errCircuitOpen = errors.New("熔断器处于打开状态")

type breakerState int

const (
	stateClosed breakerState = iota
	stateOpen
	stateHalfOpen
)

func (s breakerState) String() string {
	switch s {
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// Breaker 是一个最简熔断器：
// 连续失败 threshold 次 -> 打开；冷却 window 之后 -> 半开，只放一个探测请求；
// 探测成功 -> 关闭；探测失败 -> 重新打开。
type Breaker struct {
	mu        sync.Mutex
	state     breakerState
	failures  int
	threshold int
	window    time.Duration
	openedAt  time.Time
}

func NewBreaker(threshold int, window time.Duration) *Breaker {
	return &Breaker{threshold: threshold, window: window}
}

// Do 是调用入口：被熔断时直接返回错误，不执行 fn。
func (b *Breaker) Do(fn func() error) error {
	if err := b.beforeRequest(); err != nil {
		return err
	}
	err := fn()
	b.afterRequest(err)
	return err
}

func (b *Breaker) beforeRequest() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateOpen:
		if time.Since(b.openedAt) < b.window {
			return errCircuitOpen
		}
		b.state = stateHalfOpen // 冷却结束，放一个请求去试探
		return nil
	case stateHalfOpen:
		return errCircuitOpen // 已有探测请求在途，其余一律拒绝
	default:
		return nil
	}
}

func (b *Breaker) afterRequest(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err == nil {
		b.state = stateClosed
		b.failures = 0
		return
	}
	b.failures++
	if b.state == stateHalfOpen || b.failures >= b.threshold {
		b.state = stateOpen
		b.openedAt = time.Now()
		b.failures = 0
	}
}

func (b *Breaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.String()
}

func main() {
	br := NewBreaker(3, 100*time.Millisecond)

	upstreamDown := true
	call := func() error {
		if upstreamDown {
			return errors.New("上游连接超时")
		}
		return nil
	}

	for i := 1; i <= 4; i++ {
		err := br.Do(call)
		fmt.Printf("第 %d 次调用: err=%v 状态=%s\n", i, err, br.State())
	}

	fmt.Println("等待冷却期...")
	time.Sleep(120 * time.Millisecond)
	upstreamDown = false // 上游恢复

	err := br.Do(call)
	fmt.Printf("半开探测: err=%v 状态=%s\n", err, br.State())
	fmt.Println("恢复后连续调用:", br.Do(call), br.State())
}
```

运行结果：

```text
第 1 次调用: err=上游连接超时 状态=closed
第 2 次调用: err=上游连接超时 状态=closed
第 3 次调用: err=上游连接超时 状态=open
第 4 次调用: err=熔断器处于打开状态 状态=open
等待冷却期...
半开探测: err=<nil> 状态=closed
恢复后连续调用: <nil> closed
```

解读：

- **熔断的价值是"快速失败 + 给下游喘息"**：打开之后调用方不再发起请求，既省了自己的连接与超时等待，也给了下游恢复的机会。第 4 次调用直接返回，没有真的去连上游。
- **半开状态只放一个探测请求**，这是关键细节。若放开所有请求，恢复瞬间的流量会把刚爬起来的下游再打垮（典型的"雪崩二次伤害"）。
- **冷却时间（window）要结合下游的恢复速度来设**，通常几秒到几十秒；太短等于没熔断，太长则恢复慢。
- **生产级实现还要考虑**：滑动窗口统计（而不是简单计数）、按错误比例而非绝对次数触发、并发安全的半开探测计数、以及"熔断状态"的监控上报。成熟方案可以直接用 `sony/gobreaker` 或服务网格（熔断下沉到 Sidecar）。
- **重试 + 熔断 + 超时的正确组合顺序**：超时是底线（永远要有），重试在最外层且次数很少，熔断包住整个调用以保护下游，三者共同构成"弹性"。

### 11.4.13 并发 Map/Reduce：处理大文件

**适用场景**：单文件或单批数据的处理速度受限于单核，例如日志词频统计、大文件校验、批量数据清洗。

```go
// 摘自本章验证用例 d13_mapreduce/main.go
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	lines    = 60000
	chunkNum = 8
)

// makeWord 生成可预测的词，保证示例输出可复现
func makeWord(i int) (string, string) {
	return fmt.Sprintf("w%d", i%50), fmt.Sprintf("x%d", i%7)
}

// buildFile 在当前目录造一个大文件（用临时文件，函数返回后由调用方删除）
func buildFile() (string, error) {
	f, err := os.CreateTemp(".", "words-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 1<<20)
	for i := 0; i < lines; i++ {
		a, b := makeWord(i)
		fmt.Fprintf(w, "%s %s\n", a, b)
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// splitChunks 把文件内容切成 n 块，每块按换行对齐，避免把一个词劈成两半
func splitChunks(data []byte, n int) [][]byte {
	size := (len(data) + n - 1) / n
	var chunks [][]byte
	for start := 0; start < len(data); {
		end := min(start+size, len(data))
		if end < len(data) {
			if idx := bytes.IndexByte(data[end:], '\n'); idx >= 0 {
				end += idx + 1
			} else {
				end = len(data)
			}
		}
		chunks = append(chunks, data[start:end])
		start = end
	}
	return chunks
}

// mapChunk 是 Map 阶段：统计这一块里的词频
func mapChunk(data []byte) (map[string]int, error) {
	counts := make(map[string]int, 64)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		for _, word := range strings.Fields(sc.Text()) {
			counts[word]++
		}
	}
	return counts, sc.Err()
}

// mapReduce 并发执行 Map，再在单 goroutine 里做 Reduce
func mapReduce(ctx context.Context, data []byte, n int) (map[string]int, error) {
	chunks := splitChunks(data, n)
	partials := make([]map[string]int, len(chunks))
	errs := make([]error, len(chunks))

	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Go(func() {
			select {
			case <-ctx.Done():
				errs[i] = ctx.Err()
			default:
			}
			if errs[i] != nil {
				return
			}
			partials[i], errs[i] = mapChunk(chunk)
		})
	}
	wg.Wait()

	// 合并结果：各分片写自己的下标，彼此不共享，所以完全不需要加锁
	total := make(map[string]int)
	for i, p := range partials {
		if errs[i] != nil {
			return nil, fmt.Errorf("分片 %d: %w", i, errs[i])
		}
		for k, v := range p {
			total[k] += v
		}
	}
	return total, nil
}

func main() {
	path, err := buildFile()
	if err != nil {
		fmt.Println("造文件失败:", err)
		return
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("读文件失败:", err)
		return
	}
	fmt.Printf("文件 %s, 大小 %d 字节\n", path, len(data))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	counts, err := mapReduce(ctx, data, chunkNum)
	if err != nil {
		fmt.Println("处理失败:", err)
		return
	}

	totalTokens := 0
	for _, v := range counts {
		totalTokens += v
	}
	fmt.Println("词元总数:", totalTokens, "(期望", lines*2, ")")
	fmt.Println("不同词数:", len(counts), "(期望 57)")
	fmt.Println("w0 出现次数:", counts["w0"], "(期望", lines/50, ")")
}
```

运行结果：

```text
文件 .\words-3786723924.txt, 大小 408000 字节
词元总数: 120000 (期望 120000 )
不同词数: 57 (期望 57)
w0 出现次数: 1200 (期望 1200 )
```

（文件名里那串数字来自 `os.CreateTemp` 的随机后缀，**每次运行都不同**；文件大小与三个统计值则是确定的。）

解读：

- **切分必须按边界对齐**。按字节数硬切会把一个词劈成两半，导致词频统计出错。这里的做法是"切到目标位置后，向后找最近的换行符"。生产代码里更常见的是**按行切分**（`bufio.Scanner` 顺序读、每 K 行一个批次）或直接用 `io.ReaderAt` 分段读取。
- **每个分片写自己的下标，天然免锁**。这是并发编程里非常实用的一招：与其让 n 个 goroutine 抢同一个 map 加锁合并，不如让它们各自写 `partials[i]`，最后在单 goroutine 里顺序合并。**"分而治之 + 免共享"永远比"共享 + 加锁"更快也更不容易错**。
- **Reduce 阶段单线程**：合并 57 个 key 的开销可以忽略。如果 key 数量巨大（比如几千万），就需要考虑并发合并或分桶合并。
- **大文件不要整个读进内存**。示例为了聚焦并发结构而用了 `os.ReadFile`，真实场景应当用 `io.ReaderAt` 配合偏移量分段读，把内存占用控制在"分片大小 × 并发数"的量级。
- **并发度与磁盘**：如果瓶颈在磁盘 I/O（尤其是机械盘或云盘带宽），提高并发度反而会让总耗时变长。此时先做基准测试再决定并发数，参考 11.6.6。

💡 优缺点：优点是简单粗暴地把单核瓶颈变成多核可扩展、结果确定（易于验证）；缺点是内存占用与并发度成正比，切分逻辑有边界条件的坑，且当单核已经够快时收益为零。

## 11.5 并发错误处理

并发场景下的错误处理比串行代码麻烦，原因有三：错误来自多个 goroutine（谁先失败？要不要全失败？）、panic 在子 goroutine 里会直接终结进程、以及退出时要保证在途任务被妥善收尾。

### 11.5.1 错误收集的三种策略

**策略一：错误 channel。** 用一个容量等于任务数的缓冲 channel 收集所有错误，等所有 goroutine 结束后再关闭并集中读取。

```go
// 摘自本章验证用例 e01_errcollect/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var errFlaky = errors.New("偶发失败")

// fetch 每次都以大约 1/3 的概率失败，用来制造多个并发错误
func fetch(ctx context.Context, id int) error {
	select {
	case <-time.After(time.Duration(id%3+1) * 10 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	if id%3 == 0 {
		return fmt.Errorf("拉取 %d: %w", id, errFlaky)
	}
	return nil
}

// 策略一：错误 channel。容量等于任务数，保证 goroutine 永远不会因为
// 没人接收错误而泄漏；Wait 之后再 close 并集中读取，避免边写边读的乱序。
func collectViaChannel(ctx context.Context, ids []int) []error {
	errCh := make(chan error, len(ids))

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			if err := fetch(ctx, id); err != nil {
				errCh <- err
			}
		})
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errs
}

// 策略二：只关心第一个错误时，用 sync.Once 把"记录"这个动作做一次。
func firstError(ctx context.Context, ids []int) error {
	var (
		once  sync.Once
		mu    sync.Mutex
		first error
	)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			if err := fetch(ctx, id); err != nil {
				once.Do(func() {
					mu.Lock()
					first = err
					mu.Unlock()
				})
			}
		})
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	return first
}

// 策略三：用 errors.Join 汇总全部错误，保留完整的错误树。
func collectJoined(ctx context.Context, ids []int) error {
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() { errs[i] = fetch(ctx, id) })
	}
	wg.Wait()
	return errors.Join(errs...)
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ids := make([]int, 0, 10)
	for i := 1; i <= 10; i++ {
		ids = append(ids, i)
	}

	errs := collectViaChannel(ctx, ids)
	fmt.Println("策略一收集到错误数:", len(errs))

	first := firstError(ctx, ids)
	fmt.Println("策略二首个错误:", first, "可判定类型:", errors.Is(first, errFlaky))

	joined := collectJoined(ctx, ids)
	fmt.Println("策略三 Is 判定:", errors.Is(joined, errFlaky))

	// errors.Join 返回的错误实现了 Unwrap() []error，可以逐条处理；
	// 其中的 nil 项会被自动忽略，不必自己做过滤。
	if u, ok := joined.(interface{ Unwrap() []error }); ok {
		fmt.Println("Join 后展开的错误数:", len(u.Unwrap()))
	}
}
```

**策略二：只记第一个错误**，用 `sync.Once` 把"记录"这个动作做一次——程序中的 `firstError` 就是它。注意它抢到的"第一个"取决于时序，因此不同运行次数得到的结果可能不同。

**策略三：用 `errors.Join` 汇总全部错误**，保留完整错误树——程序中的 `collectJoined` 就是它。各分片写自己的下标，因此完全不需要加锁。

运行结果：

```text
策略一收集到错误数: 3
策略二首个错误: 拉取 6: 偶发失败 可判定类型: true
策略三 Is 判定: true
Join 后展开的错误数: 3
```

解读：

- **错误 channel 的容量必须是任务数**（或者至少保证接收方一定在消费）。如果容量不足、又没有专职的接收 goroutine，发送方会永久阻塞，`wg.Wait()` 也就永不返回——这是把"错误收集"写成死锁的经典方式。
- **"等所有 goroutine 结束再 close 并集中读取"** 比"边写边读"更简单也更安全，代价是错误要占一点内存缓冲。任务量极大（百万级）时应该改成"专职收集 goroutine + `select` 监听 ctx"。
- **策略二的"第一个错误"是不确定的**。输出里是"拉取 6"，但它取决于哪个任务先失败——不同机器、不同运行次数结果都不同。如果业务上需要稳定的"优先级最高的错误"，就不能靠 `sync.Once` 抢，而要显式定义优先级规则（例如"参数错误 > 超时 > 其他"）。
- **`errors.Join` 忽略 nil**，所以直接把 `errs` 整个切片丢进去即可，不必自己过滤。它的返回值实现了 `Unwrap() []error`，可以逐条取出做分类统计（示例最后一行打印了展开后的条目数）。
- **`errs[i] = fetch(...)` 这种"各写各的下标"同样是免锁的**，与 11.4.13 的思路一致。

### 11.5.2 panic 在 goroutine 中的处理范式

必须记住一条铁律：**任何一个 goroutine 里未被 recover 的 panic 都会终结整个进程**，而且它的堆栈里不会包含"父 goroutine"的信息——排查时你会只看到一个孤立的崩点。

```go
// 摘自本章验证用例 e02_panicrecover/main.go
package main

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

var errWorkerPanic = errors.New("worker 内部 panic")

// Go 把一个函数放进 goroutine，并把 panic 转换成 error 回传。
// 注意 errc 必须是带缓冲的：panic 时只发送一次，
// 缓冲为 1 保证发送方不会因为没人接收而永久阻塞。
func Go(fn func() error) <-chan error {
	errc := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 一定要带上堆栈：只记录 "panic: xxx" 几乎无法定位问题
				errc <- fmt.Errorf("%w: %v\n%s", errWorkerPanic, r, debug.Stack())
			}
		}()
		errc <- fn()
	}()
	return errc
}

func risky(id int) error {
	if id == 3 {
		panic(fmt.Sprintf("id=%d 触发空指针解引用", id))
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}

func main() {
	// 1) 单个 goroutine 的 panic 被转换成 error
	err := <-Go(func() error { return risky(3) })
	fmt.Println("是否 panic 转成的错误:", errors.Is(err, errWorkerPanic))
	fmt.Println("错误首行:", strings.SplitN(err.Error(), "\n", 2)[0])
	fmt.Println("是否携带堆栈:", strings.Contains(err.Error(), "goroutine"))

	// 2) 与 errgroup 结合：个别任务崩溃不会拖垮整个进程，
	//    这是"批量任务里坏一个不能影响整体"的常用做法。
	g := new(errgroup.Group)
	g.SetLimit(3)
	for i := 1; i <= 6; i++ {
		id := i
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("worker %d: %w: %v", id, errWorkerPanic, r)
				}
			}()
			return risky(id)
		})
	}
	fmt.Println("errgroup 汇总:", strings.SplitN(fmt.Sprint(g.Wait()), "\n", 2)[0])

	// 3) 用 WaitGroup.Go 时必须自己在函数内部 recover：
	//    WaitGroup.Go 不会吞掉 panic，它会重新 panic 出去，
	//    因此整个进程依然会崩溃。
	var wg sync.WaitGroup
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("在函数内部自行兜住了 panic:", r)
			}
		}()
		panic("手动 recover 演示")
	})
	wg.Wait()
	fmt.Println("主流程安然无恙")
}
```

运行结果：

```text
是否 panic 转成的错误: true
错误首行: worker 内部 panic: id=3 触发空指针解引用
是否携带堆栈: true
errgroup 汇总: worker 3: worker 内部 panic: id=3 触发空指针解引用
在函数内部自行兜住了 panic: 手动 recover 演示
主流程安然无恙
```

解读：

- **`recover()` 必须在 `defer` 的函数里直接调用**，隔了一层函数调用就失效（`defer recover()` 这种写法完全无效，`recover` 的返回值也会被丢弃）。
- **把 panic 转成 error 时一定要带堆栈**（`debug.Stack()`）。否则日志里只有"panic: runtime error: invalid memory address"，你无法知道崩在哪一行。
- **`errc` 的缓冲必须是 1**。若用无缓冲 channel 而调用方已经放弃等待，panic 分支的发送会永久阻塞——那时候你也得不到任何信息。这与 11.5.1 中"缓冲容量"的教训是同一个。
- **给每个后台 goroutine 套一层"panic → error"的包装**，是服务端代码的常规操作。它能把"个别任务崩了导致整机挂掉"降级为"个别任务失败，进入重试或告警"。
- **`sync.WaitGroup.Go` 不会吞掉 panic**：它内部先 `recover`、再 `panic(x)` 原样抛出，因此**整个进程依然会崩溃**。要用 `WaitGroup.Go` 就必须自己在函数体内部 `recover`（示例第 3 段演示了这一点）。
- **`net/http` 的 handler 是个例外**：标准库的 server 已经为每个连接做了 `recover`（并打印堆栈），所以 handler 里的 panic 不会直接终结进程——但会中断这次请求。**自己起的 goroutine 没有这层保护。**
- 反面写法（片段，仅供对照，不要照抄）：

```go
go func() {
	// 危险：没有任何 recover，这个 panic 会终结整个进程
	process(job)
}()
```

### 11.5.3 优雅退出（Graceful Shutdown）

优雅退出要解决四件事：**停止接受新请求、等待在途请求完成、给等待设一个上限、释放资源**。标准的实现是 `signal.NotifyContext` + `http.Server.Shutdown`：

```go
// 摘自本章验证用例 e03_graceful/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 1) 用 signal.NotifyContext 把信号变成 ctx 取消：
	//    收到 SIGINT / SIGTERM 后 ctx.Done() 关闭，所有监听它的 goroutine 一起收敛。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2) 监听 127.0.0.1:0，由系统分配端口，避免端口冲突
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("监听失败:", err)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		// 用请求自带的 ctx：客户端断开或服务关闭都能感知
		select {
		case <-time.After(300 * time.Millisecond):
			io.WriteString(w, "slow response\n")
		case <-r.Context().Done():
			fmt.Println("[server] 请求被中断:", r.Context().Err())
		}
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second, // 防慢速攻击的必备项
	}

	serveErr := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	addr := ln.Addr().String()
	fmt.Println("[server] 监听", addr)

	// 3) 造一个在途请求，用来验证优雅关闭确实会等它跑完
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			fmt.Println("[client] 请求失败:", err)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Print("[client] 收到: ", string(body))
	}()
	time.Sleep(50 * time.Millisecond) // 等请求真正打进来

	// 真实服务在这里等信号；示例改用定时器模拟收到信号，保证程序能自己退出
	select {
	case <-ctx.Done():
		fmt.Println("[server] 收到系统信号")
	case <-time.After(80 * time.Millisecond):
		fmt.Println("[server] 模拟收到 SIGTERM")
	}

	// 4) 关闭流程：给在途请求留出收尾时间，到点则强制关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Println("[server] 优雅关闭失败:", err)
	}
	fmt.Println("[server] 在途请求已收尾，耗时", time.Since(start).Round(10*time.Millisecond))

	// 关闭之后新连接会被拒绝
	if _, err := http.Get("http://" + addr + "/slow"); err != nil {
		fmt.Println("[server] 关闭后新请求被拒绝:", err != nil)
	}

	select {
	case err := <-serveErr:
		fmt.Println("[server] Serve 返回异常:", err)
	default:
	}
	time.Sleep(100 * time.Millisecond) // 让 client goroutine 打印完
}
```

运行结果：

```text
[server] 监听 127.0.0.1:13950
[server] 模拟收到 SIGTERM
[client] 收到: slow response
[server] 在途请求已收尾，耗时 270ms
[server] 关闭后新请求被拒绝: true
```

（监听端口来自 `net.Listen("tcp", "127.0.0.1:0")` 的**系统分配**，每次运行都不同；实测得到过 `127.0.0.1:5091`、`5094`、`5097`。最后那行耗时也随调度在 260~270ms 之间浮动——它约等于 handler 里那 300 毫秒减去请求已经跑掉的部分。）

解读：

- **`signal.NotifyContext` 把信号统一进 Context 体系**：这是 Go 1.16+ 的标准做法，比手工 `signal.Notify` + channel 少写十几行，而且天然能级联取消所有后台 goroutine。
- **`srv.Shutdown` 的语义是"停止接收新连接 + 等已有连接处理完"**。输出里在途请求耗时 270 毫秒、`Shutdown` 也等了约 270 毫秒，之后新请求才被拒绝——这正是我们要的顺序：先排空，再关闭。
- **`Shutdown` 的 ctx 必须有超时**（这里是 3 秒）。没有超时的话，一个卡住的请求会让进程永远退不出去，最终只能被 `kill -9`，反而丢掉了优雅退出的意义。超时后应调用 `srv.Close()` 强制关闭。
- **handler 里要用 `r.Context()`**，它同时承载"客户端断开"和"服务器关闭"两种取消。示例中如果客户端提前断开，handler 会从 `r.Context().Done()` 分支退出，不再白白占用资源。
- **完整的退出流程还有一个环节**：`Shutdown` 只负责 HTTP 层，其他后台 goroutine（定时任务、消费者、metrics 上报）也要监听同一个 ctx 并自行收尾，通常再用一个 `WaitGroup` 等它们结束，最后才让 `main` 返回。
- **`http.Server` 的超时字段值得配齐**：`ReadHeaderTimeout`（防慢速攻击）、`ReadTimeout`、`WriteTimeout`、`IdleTimeout`。只设 `ReadHeaderTimeout` 就能挡掉相当一部分攻击流量，属于性价比最高的配置。

## 11.6 并发性能与调优

### 11.6.1 锁竞争分析：mutex profile 与 block profile

并发程序变慢，十有八九是**锁竞争**或**阻塞**。Go 运行时内置了两种采样：mutex profile 记录"谁在等锁、等了多久"，block profile 记录"goroutine 因为同步原语阻塞了多久"。

在程序里打开采样（生产环境建议采样率不要用 1，避免开销）：

```go
// 摘自本章验证用例 f03_profile/main.go
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"
)

func main() {
	// 1) 打开互斥锁采样与阻塞采样。
	//    采样率是"每 n 个事件采一次"，1 表示全采（仅供演示），生产环境用较大值降低开销。
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	// 2) 同时打开执行跟踪，它记录的是"时间轴上发生了什么"，与 pprof 是互补关系
	traceFile, err := os.Create("trace.out")
	if err != nil {
		fmt.Println("创建 trace 文件失败:", err)
		return
	}
	if err := trace.Start(traceFile); err != nil {
		fmt.Println("启动 trace 失败:", err)
		return
	}

	// 3) 制造锁竞争：4 个 goroutine 抢同一把锁，临界区里还有一段耗时操作
	var mu sync.Mutex
	var counter int64
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 20000 {
				mu.Lock()
				time.Sleep(50 * time.Microsecond) // 模拟临界区里的"顺手多做了点事"
				counter++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	trace.Stop()
	traceFile.Close()

	fmt.Println("counter =", counter)

	// 4) 把 mutex / block profile 落盘，供 go tool pprof 分析
	for _, name := range []string{"mutex", "block"} {
		p := pprof.Lookup(name)
		if p == nil {
			continue
		}
		f, err := os.Create(name + ".pprof")
		if err != nil {
			fmt.Println("创建文件失败:", err)
			continue
		}
		// debug = 0 写出二进制格式（给 go tool pprof 用）
		if err := p.WriteTo(f, 0); err != nil {
			fmt.Println("写出 profile 失败:", err)
		}
		f.Close()
		fmt.Println("已写出", name+".pprof")
	}

	// 5) 程序内也可以直接读采样结果，便于做自动化阈值告警
	if p := pprof.Lookup("mutex"); p != nil {
		fmt.Println("mutex profile 采样条目数:", p.Count())
	}
}
```

```bash
# 方式一：在程序里用 runtime/pprof 落盘，事后分析
go tool pprof -top mutex.pprof
go tool pprof -http=:8080 mutex.pprof      # 图形界面（火焰图/调用图/源码）

# 方式二：引入 net/http/pprof 后在线采样（注意做好鉴权与内网隔离）
# import _ "net/http/pprof" 并启动 http.ListenAndServe("localhost:6060", nil)
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/mutex
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/block
```

下面是本章验证用例在一个 4 核并发争锁的负载下，用 `go tool pprof -top mutex.pprof` 得到的真实输出（已截断。**注意：绝对数值与机器、运行次数强相关**——本书复核时在同一份程序上得到的是 `Showing nodes accounting for 196.87s ... / Dropped 29 nodes (cum <= 0.98s)`，三行符号名与结构与下面完全一致，请只把数值当量级参考）：

```text
Type: delay
Showing nodes accounting for 173.62s, 100% of 173.62s total
Dropped 40 nodes (cum <= 0.87s)
      flat  flat%   sum%        cum   cum%
   173.62s   100%   100%    173.62s   100%  sync.(*Mutex).Unlock (inline)
         0     0%   100%    173.62s   100%  main.main.func1
         0     0%   100%    173.62s   100%  sync.(*WaitGroup).Go.func1
```

解读：

- **`Type: delay`** 表示这是"等待时长"profile。mutex profile 统计的是"因争锁而等待的累计时间"，因此数值大本身就是信号：**这个总和是 4 个 goroutine 的等待时间之和，不是墙钟时间**，它远大于墙钟运行时长，说明这 4 个 goroutine 大部分时间都在排队。另外提醒一句：这份示例把 `SetBlockProfileRate(1)` 打开到了全采（每 50 微秒的临界区都要记一次事件），实测跑完需要几十秒，**这正说明采样率绝不能在真实环境里这么设**。
- **`sync.(*Mutex).Unlock` 出现在热点上**是正常现象：Go 的 mutex 把"唤醒等待者"的工作放在 `Unlock` 里（因为解锁的那个 goroutine 反正要持有锁），所以竞争的代价记在 `Unlock` 上，而不是 `Lock` 上。**不要被这个符号名误导**，它就是"锁竞争"的直接证据。
- **`flat` 与 `cum` 的区别**：`flat` 是函数自身消耗的时间，`cum` 是包含被调用者的累计时间。找锁竞争主要看 `flat` 高的那一行。
- **看 profile 的正确姿势**：先用 `-top` 找到热点函数，再用 `-list 函数名` 看具体哪一行，最后用 `-http` 看调用图或火焰图。**先定位，再优化**，不要凭直觉改并发度。
- **`runtime.SetMutexProfileFraction(n)` 的 `n`** 表示"每 n 个事件采样一次"，0 表示关闭；生产环境常用 5~100。`runtime.SetBlockProfileRate(rate)` 的参数是纳秒，表示"阻塞超过该时长才记录"。两者都会带来额外开销，**只在排查期打开**。

### 11.6.2 runtime/trace：时间轴视角

pprof 回答"谁慢"，`runtime/trace` 回答"这一秒里，每个 goroutine 在干什么"。它把调度、GC、系统调用、网络阻塞、同步阻塞都记录到一条时间轴上。

打开方式就是在被测代码前后各加一行：`trace.Start(traceFile)` 开始记录（上一节那份完整程序的第 2 步就是这么做的），`trace.Stop()` 结束记录；之后用下面的命令离线查看。

```bash
go tool trace trace.out
# 或者直接对运行中的服务抓取 5 秒
curl -o trace.out http://localhost:6060/debug/pprof/trace?seconds=5
go tool trace trace.out
```

打开之后重点看这几个视图：

- **View trace**：全屏时间轴。每一行是一个 P（不是 goroutine），可以看到 GC 的 STW 区间、每个 P 上运行的 goroutine 块、以及"G 处于可运行但没被调度"的延迟。
- **Goroutine analysis**：按函数归类统计 goroutine，能一眼看出"某类 goroutine 数量异常增长"，非常适合定位泄漏。
- **Scheduler latency profile**：goroutine 从"变成可运行"到"真正开始跑"的等待时间分布。这个值是**调度真实开销**的直接度量：如果它很高，说明要么并发度太高、要么有大量阻塞操作。
- **Sync blocking profile / Network blocking profile / Syscall blocking profile**：分别对应锁与 channel 阻塞、网络等待、系统调用阻塞——正是把 pprof 的"慢"拆解成"为什么慢"的地方。

💡 提示：`trace.Start` 期间会记录大量事件，**不要长期开启**；典型做法是在压测或线上取样 3~10 秒，然后离线分析。另外 `trace` 与 `pprof` 可以同时开，两者互补。

### 11.6.3 减少锁竞争的四条路

从上到下依次尝试，收益通常就越大：

**第一条：分片（sharding）。** 把一个锁保护的全局结构拆成 N 份，各 goroutine 尽量访问不同的分片。

```go
// 摘自本章验证用例 f01_shards/main.go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	workers   = 8
	perWorker = 200_000
)

// 1) 单锁计数器：全部 goroutine 争抢同一把锁
type mutexCounter struct {
	mu sync.Mutex
	n  int64
}

func (c *mutexCounter) Inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *mutexCounter) Sum() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// 2) 分片计数器：每个 worker 固定用自己那一片，锁几乎不冲突
type shardedCounter struct {
	shards [16]struct {
		mu sync.Mutex
		n  int64
	}
}

func (c *shardedCounter) Inc(id int) {
	s := &c.shards[id%len(c.shards)]
	s.mu.Lock()
	s.n++
	s.mu.Unlock()
}

func (c *shardedCounter) Sum() int64 {
	var total int64
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		total += s.n
		s.mu.Unlock()
	}
	return total
}

// 3) 原子计数器：无锁，单变量场景下通常最快
type atomicCounter struct {
	n atomic.Int64
}

func (c *atomicCounter) Inc()       { c.n.Add(1) }
func (c *atomicCounter) Sum() int64 { return c.n.Load() }

// bench 跑一个固定工作量的并发计数，返回耗时
func bench(name string, inc func(id int), sum func() int64) {
	var wg sync.WaitGroup
	start := time.Now()
	for id := range workers {
		wg.Go(func() {
			for range perWorker {
				inc(id)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("%-12s 耗时 %-12v 结果 %d\n", name, elapsed.Round(time.Millisecond), sum())
}

func main() {
	mc := &mutexCounter{}
	sc := &shardedCounter{}
	ac := &atomicCounter{}

	// 注意：这里的绝对耗时与机器强相关，只比较三者的相对量级
	bench("单锁", func(int) { mc.Inc() }, mc.Sum)
	bench("分片锁", func(id int) { sc.Inc(id) }, sc.Sum)
	bench("原子量", func(int) { ac.Inc() }, ac.Sum)

	expect := int64(workers * perWorker)
	fmt.Println("三种实现结果一致:", mc.Sum() == expect && sc.Sum() == expect && ac.Sum() == expect)
}
```

**第二条：读写锁。** 读多写少的场景，把 `sync.Mutex` 换成 `sync.RWMutex`，让多个读者并行。

**第三条：原子操作。** 单变量计数、标志位这类"简单状态"，直接用 `sync/atomic`。

**第四条：无锁结构与 CAS 重试。** 用 `CompareAndSwap` 循环实现无锁更新，或者干脆改写数据流（例如 11.4.13 的"各写各的下标"）。

本章验证用例用 `go test -bench` 得到的真实结果如下（`-benchtime 300000x`，机器为 20 逻辑核，`-20` 表示并行度等于 GOMAXPROCS，也就是每个核都在抢锁）：

```text
BenchmarkMutexCounter-20      	  300000	        68.14 ns/op
BenchmarkShardedCounter-20    	  300000	        25.30 ns/op
BenchmarkAtomicCounter-20     	  300000	        19.20 ns/op
```

解读：

- **数量级关系比绝对值重要**：单锁约 68 纳秒/次，分片锁约 25，原子量约 19。单锁→分片的提升（约 2~3 倍）来自"不再争抢同一缓存行上的锁变量"；分片→原子量的提升较小，因为分片锁在无冲突时也要执行两次原子指令加一次解锁通知。
- **注意前提**：分片版之所以好看，是因为每个 goroutine 固定访问自己的分片（`Inc(id)` 里的 `id` 由调用方保证分散）。如果所有 goroutine 都随机访问分片，命中同一分片的概率回升，收益会明显缩水。**分片键的选择决定一切。**
- **不要盲目上原子操作**：`atomic` 只能保护单个操作，任何"读—改—写"的组合逻辑（例如 `if x < limit { x++ }`）都必须用 CAS 循环或锁。把它当计数器用最合适。
- **基准测试要防优化**：`b.RunParallel` 是并发基准的标准入口；如果被测函数是纯计算且结果未被使用，编译器可能把它优化掉，必要时把结果写入一个 `atomic` 变量或 `Sink`。

### 11.6.4 channel 与 mutex：什么时候用哪个

这是 Go 社区讨论最多的话题。结论可以总结成一张表：

| 维度 | channel | mutex |
| --- | --- | --- |
| 主要用途 | 传递数据的所有权、编排 goroutine 生命周期、等待事件 | 保护共享状态（结构体、map、缓存、计数器） |
| 性能（无竞争） | 约几十到一两百纳秒（含调度） | 约十几纳秒（未竞争时是一条原子指令） |
| 性能（高竞争） | 退化为排队，延迟随队列变长 | 自旋 + 排队，也可通过分片缓解 |
| 表达能力 | 强：可 `select` 多路复用、可关闭广播、可带缓冲做背压 | 弱：只有"进入/离开" |
| 错误风险 | 泄漏 goroutine、死锁、向已关闭 channel 发送 | 忘记解锁、复制了含锁的结构体、锁顺序不当 |
| 典型反例 | 用 channel 保护一个计数器 | 用 mutex 做复杂的 goroutine 编排 |

一句话原则：**"传递数据用 channel，保护状态用 mutex。"** 再补两条实践规律：

- **channel 的成本主要在调度**。每次跨 goroutine 传递都可能触发调度器介入，所以在"高频、极短临界区"的路径上（比如一个每秒被调用千万次的计数器），mutex 或 atomic 通常更快。基准测试仍然是最终裁判。
- **不要为了"纯 Go 风格"而绕过 mutex**。`sync.Map`、`sync.Pool`、`sync.Once` 这些工具本身就是官方对"channel 不是万能"的回答。

### 11.6.5 假共享（False Sharing）与缓存行填充

假共享是"两个 goroutine 明明访问不同变量，却互相拖慢"的现象。原因是 CPU 以**缓存行**（cache line，通常 64 字节）为单位在核心之间同步数据：

```text
   无填充（假共享）：
   缓存行 0x00 ~ 0x3F（64 字节）
   +----------+----------+---------------------------+
   |    a     |    b     |      （同一行的其他数据）    |
   +----------+----------+---------------------------+
        ^          ^
        |          |
     M0 写 a     M1 写 b
   => 同一缓存行的所有权在两个核心之间来回弹跳（cache line ping-pong），
      每次写都要把对方的缓存副本置为失效，性能随核数增加而恶化。

   加了填充（padding）：
   缓存行 0x00 ~ 0x3F                 缓存行 0x40 ~ 0x7F
   +----------+----------------+     +----------+----------------+
   |    a     |  56 字节填充    |     |    b     |  56 字节填充    |
   +----------+----------------+     +----------+----------------+
   => 两个变量各占一行，互不干扰。
```

```go
// 摘自本章验证用例 f02_padding/main.go
package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

// 两个热点字段紧挨着，很可能落在同一条 64 字节缓存行里
type noPad struct {
	a atomic.Int64
	b atomic.Int64
}

// 用显式填充把两个字段推离同一条缓存行：
// 8 字节的 a + 56 字节填充 = 64 字节，b 恰好从下一条缓存行开始
type padded struct {
	a atomic.Int64
	_ [56]byte
	b atomic.Int64
}

func main() {
	var x noPad
	var y padded

	fmt.Printf("noPad  大小=%d  a 偏移=%d  b 偏移=%d\n",
		unsafe.Sizeof(x), unsafe.Offsetof(x.a), unsafe.Offsetof(x.b))
	fmt.Printf("padded 大小=%d  a 偏移=%d  b 偏移=%d\n",
		unsafe.Sizeof(y), unsafe.Offsetof(y.a), unsafe.Offsetof(y.b))

	// 两字段起点之间的距离：小于 64 字节就有落在同一缓存行的风险
	fmt.Println("noPad 中两字段间距:", unsafe.Offsetof(x.b)-unsafe.Offsetof(x.a), "字节（远小于 64，同一缓存行）")
	fmt.Println("padded 中两字段间距:", unsafe.Offsetof(y.b)-unsafe.Offsetof(y.a), "字节（正好跨缓存行）")

	// 填充的代价：结构体变大，缓存利用率下降，只在确实热点时才加
	fmt.Println("填充带来的额外开销:", unsafe.Sizeof(y)-unsafe.Sizeof(x), "字节")

	// 顺带演示：atomic.Int64 自身就是一个结构体，占 8 字节且 8 字节对齐
	var z atomic.Int64
	fmt.Println("atomic.Int64 大小:", unsafe.Sizeof(z))
}
```

运行结果（用 `unsafe` 直接验证布局）：

```text
noPad  大小=16  a 偏移=0  b 偏移=8
padded 大小=72  a 偏移=0  b 偏移=64
noPad 中两字段间距: 8 字节（远小于 64，同一缓存行）
padded 中两字段间距: 64 字节（正好跨缓存行）
填充带来的额外开销: 56 字节
atomic.Int64 大小: 8
```

对应的基准测试（`go test -bench . -cpu 4`）让 4 个并行 goroutine 按编号奇偶分别去写 `a` 与 `b`：

```text
BenchmarkFalseSharing-4   	 2000000	        12.48 ns/op
BenchmarkPadded-4         	 2000000	         3.598 ns/op
```

解读：

- **收益很实在**：未填充约 12.5 纳秒/次，填充后约 3.6 纳秒/次，差了三倍多。在"每核一个计数器"这类结构（比如 per-P 统计、分片计数器的分片本身）上，这个优化经常是决定性的。
- **`_ [56]byte` 是唯一的表达方式**。Go 没有 `alignas`，也没有"缓存行大小"这个编译期常量（`unsafe.Sizeof` 只能算类型大小，`cpu.CacheLinePad` 之类的常量来自第三方库）。所以填充长度是按 64 字节假定的，**在缓存行不是 64 字节的架构上会失效**——这属于"已知硬件假设"，要在注释里写清楚。
- **填充不是免费的**：结构体从 16 字节膨胀到 72 字节，缓存里能放的对象变少，序列化/网络传输也可能变大（庆幸的是，`_ [56]byte` 不会被导出字段的编码器写出，但整体内存占用是实打实的）。所以**只对真正的热点字段做填充**，不要全局滥用。
- **先测量再填充**：如果 profile 显示热点集中在两个相邻字段的写操作上，且有明显随核数恶化的现象，才考虑填充。**绝大多数业务代码根本不需要它**，这是典型的"知道就好，别乱用"的优化。

### 11.6.6 GOMAXPROCS、批处理与避免过度并发

**GOMAXPROCS 不是越大越好。** 它的作用是把 goroutine 映射到可运行的 P 上，超过 CPU 可承载的并发度只会增加调度开销与缓存颠簸（cache thrash）。三条实践建议：

1. **CPU 密集任务**：让 `GOMAXPROCS` 保持在可用 CPU 数附近（Go 1.25+ 默认已经是"逻辑 CPU、亲和性、cgroup 配额"三者取最小，通常无需手工设置）。
2. **I/O 密集任务**：`GOMAXPROCS` 可以略低于 CPU 数也没关系，真正的并发度由**阻塞型操作的数量**决定；此时限制并发度的应该是信号量或 worker pool，而不是 GOMAXPROCS。
3. **容器环境**：确认默认值是否与 CPU limit 一致（见 11.2.8）；如果容器配额是 2 核而程序认为自己有 64 核，性能会断崖式下跌。

**批处理降低调度开销。** 一个高频循环里"每个元素起一个 goroutine"是最常见的性能自杀方式（片段）：

```go
// 差：100 万个元素 => 100 万个 goroutine + 100 万次 channel 往返
for _, item := range items {
	go process(item)
}

// 好：按批次分发，批次内顺序处理，减少调度与同步开销
const batch = 256
for start := 0; start < len(items); start += batch {
	end := min(start+batch, len(items))
	go processBatch(items[start:end])
}
```

批处理的好处有三点：goroutine 数量下降几个数量级、每次通信的摊销成本降低、缓存局部性变好。代价是**延迟略增**（一个批次要等最慢的那个）和**负载不均**（批次大小应与任务耗时分布匹配）。

**避免过度并发的三个信号**：

- `schedtrace` 里 `spinningthreads` 长期大于 0，而 `runqueue` 也很长——说明大量 goroutine 在抢有限的 P，纯粹在浪费 CPU。
- `runtime/trace` 的 Scheduler latency profile 显示调度等待时间与计算时间同量级。
- mutex/block profile 显示大量时间花在"等"上，而不是花在"算"上。

遇到这些信号，正确的方向通常是**减少并发度、加批处理、优化临界区**，而不是继续加机器或加 goroutine。

## 11.7 并发编程检查清单

下面这 20 条可以打印出来贴在工位上，提交并发代码前逐条对照。它们全部来自本章讨论过的原理与坑，每一条都能追溯到具体的一个小节。

**关于 goroutine 的生命周期**

1. 每个 goroutine 是否都有**明确的退出路径**？（不能存在"只能靠进程退出"的 goroutine）
2. 每个长时间运行的 goroutine 是否监听 `ctx.Done()` 或一个专门的 `quit` channel？
3. 循环里的发送/接收是否都挂上了取消分支（`case out <- v:` 与 `case <-ctx.Done():` 成对出现）？
4. 是否用 `runtime.NumGoroutine()` 或 `runtime/trace` 验证过"函数返回后 goroutine 数量回落到基线"？

**关于 channel**

5. 每个 channel 是否**由发送方关闭**（且只关一次）？
6. 是否避免了"向已关闭的 channel 发送"与"重复关闭"这两种 panic？
7. 消费者是否使用 `for v := range ch` 或检查了 `ok`，以正确处理"关闭后读到零值"？
8. channel 的缓冲容量是否有明确理由（背压？削峰？性能？），而不是随手写一个数字？

**关于 Context**

9. `context.Context` 是否是函数的**第一个参数**、命名为 `ctx`，且没有被塞进结构体？
10. 每个 `WithCancel` / `WithTimeout` / `WithDeadline` 是否都配了 `defer cancel()`？
11. 中间层是否只做**透传**，没有偷偷换成 `Background()`（超时链路是否完整）？
12. 取消原因的判定是否用了 `errors.Is(err, context.Canceled/DeadlineExceeded)`，而不是字符串比较？

**关于共享状态**

13. 每一次对共享变量的读写，能否指认出建立 happens-before 的那一行代码？
14. 锁的**加锁顺序**在所有代码路径上是否一致（避免死锁）？
15. `sync.Mutex` 是否作为值被复制过（含锁的结构体不要按值传递，`go vet` 的 `copylocks` 能查出来）？
16. 临界区是否足够小——里面是否误放了 I/O、网络调用、`time.Sleep` 甚至另一个 channel 操作？

**关于错误与退出**

17. 每个后台 goroutine 是否都有 `recover` 并把 panic 转成 error 上报（或确认它处于 `net/http` 的保护之下）？
18. 并发任务的错误是否被收集并处理，而不是被丢弃在无人接收的 channel 里？
19. 服务是否实现了优雅退出：`signal.NotifyContext` → 停止接收新请求 → 等待在途任务（带超时）→ 释放资源？

**关于验证与观测**

20. 是否用 `go test -race ./...` 跑过测试？是否在 CI 里有至少一台能跑 `-race` 的机器？关键并发路径是否有用 `-count=100` 或多轮重复运行来暴露偶发问题？

补充三条"上线前"的自查项，虽然不在 20 条之内，但同样值得一查：**是否为所有外部调用都设了超时**；**是否给下游并发加了信号量或限流**（防止自己的重试把对方打垮）；**是否有 `schedtrace` / `trace` / mutex profile 的采样手段**，以便出问题时能定位而不是猜。

## 11.8 本章小结

- **Context 的本质是"沿调用链传递的取消信号 + 截止时间 + 请求域元数据"**，它是不可变、并发安全的，每次派生都产生一个新节点，所有节点构成一棵树，取消沿树向下级联。
- **四个方法各有分工**：`Deadline` 用于预算分配，`Done` 用于等待与 select，`Err`/`Cause` 用于判定原因，`Value` 用于请求域元数据。取消的判定必须用 `errors.Is`。
- **`cancel()` 必须调用**（优先 `defer cancel()`），否则子节点会挂在父节点上、定时器也不会释放，在长生命周期的父 Context 下就是内存泄漏。`go vet` 的 `lostcancel` 检查专门抓这个。
- **`WithValue` 只放请求域元数据**：自定义 key 类型避免冲突，值要做带 ok 的类型断言，读写收口成一对函数；不要放可选参数、日志器、数据库连接或业务对象。
- **8 条 Context 规范**中最容易违反的是"中间层不要换成 `Background()`"与"不要塞进结构体"，前者会切断超时链路，后者会混淆生命周期。
- **Go 1.20/1.21 补齐了取消语义**：`WithCancelCause` + `Cause` 让取消携带原因，`WithTimeoutCause` 让超时也有原因，`WithoutCancel` 切断取消但保留值，`AfterFunc` 注册取消回调（回调在独立 goroutine 执行，需要 `stop` 来撤销）。
- **GMP 的核心约束是"M 必须持有 P 才能执行 G"**，因此真正的并行度由 `GOMAXPROCS`（P 的数量）决定，而不是 goroutine 数量。
- **两级运行队列是为了减少锁竞争**：每个 P 有 256 容量的本地队列加一个 `runnext` 插队位，全局队列负责兜底与公平（每 61 次调度取一批）。
- **work stealing 与 hand off 是调度器的两个自适应机制**：前者让空闲 P 偷一半任务，后者让阻塞在系统调用里的 M 把 P 交出去，避免稀缺的 P 被睡着的线程占住。
- **抢占式调度在 1.14 从"协作式栈检查"升级为"基于信号的异步抢占"**（Unix 上的 `SIGURG`），解决了"没有函数调用的死循环无法被抢占、进而拖死 STW"的老问题。
- **goroutine 的栈可增长、可移动**：初始约 2 KB，触发增长时运行时把旧栈整体复制并修正所有指向栈内的指针；C 线程栈做不到，因为 C 无法精确定位并改写这些指针。
- **`GOMAXPROCS` 在 Go 1.25+ 已原生感知容器**：默认取逻辑 CPU、CPU 亲和性掩码、cgroup CPU 配额三者的最小值（非整数配额向上取整），并每秒自动更新；`GODEBUG=containermaxprocs=0` 与 `updatemaxprocs=0` 可分别关闭这两项行为。
- **`GODEBUG=schedtrace=1000` 是零成本的调度器观测入口**：重点看 `idleprocs`、`runqueue`、每个 P 的队列长度分布与 `schedticks` 是否均衡；加 `scheddetail=1` 可看到每个 P/M/G 的细节。
- **Go 的内存模型用 happens-before 定义同步**：goroutine 内部顺序、`go` 语句 → 新 goroutine、channel 发送/接收与 close、Mutex 的 Unlock → Lock、Once、WaitGroup 的 Done → Wait、atomic 的顺序一致性，这些是仅有的"顺序保证"来源。
- **goroutine 退出不建立任何同步**，"起一个 goroutine 写变量、主协程睡一会儿再读"是错的：每一次共享访问都要能指出建立 happens-before 的那一行代码。
- **Go 的 atomic 只有一档强度**，等价于 C++ 的 `seq_cst` 与 Java 的 `volatile`，因此不存在"原子性对了但可见性错了"的坑；代价是弱内存序架构上需要内存屏障。
- **同步 = 原子性 + 可见性 + 顺序性**，三者缺一不可；`atomic` 只保护它自己那一个字，不保护你真正关心的数据，除非两者之间存在 happens-before。
- **13 个并发模式覆盖了绝大多数场景**：Generator、Fan-Out/Fan-In、Pipeline、Worker Pool、Semaphore、Rate Limiter、Or-Done/Tee/Bridge、Future、ErrGroup、Singleflight、Pub/Sub、重试与熔断、并发 Map/Reduce。每个模式都要记住它"解决什么问题"和"代价是什么"。
- **`errgroup` 是并发任务的标准答案**：`WithContext` 联动取消、`SetLimit` 限制并发、`Wait` 只返回第一个错误；但取消不会杀死 goroutine，任务必须自己监听 ctx。
- **`singleflight` 治的是缓存击穿**：二次缓存检查必须写在 `Do` 的函数体内部，否则回源次数不会收敛到 1；它只合并单进程内的并发请求。
- **panic 处理是并发代码的必答题**：每个后台 goroutine 都要 `recover` 并用 `debug.Stack()` 把堆栈转成 error；`WaitGroup.Go` 不会吞掉 panic，而 `net/http` 的 handler 有标准库保护。
- **优雅退出是四步**：监听信号（`signal.NotifyContext`）→ 停止接收新请求（`Shutdown`）→ 带超时地等待在途任务 → 释放资源，并让所有后台 goroutine 都监听同一个 ctx。
- **调优的顺序永远是"观测 → 定位 → 验证"**：`go tool pprof` 看 mutex/block profile 定位锁竞争，`go tool trace` 看调度延迟与阻塞原因，再用基准测试验证改动；减少竞争的手段依次是分片、读写锁、原子操作、无锁结构。
- **channel 传递数据，mutex 保护状态**：无竞争时 mutex 更快（十几纳秒量级），channel 的成本主要在调度；高频极短临界区优先考虑 mutex 或 atomic，复杂编排优先考虑 channel。
- **假共享是真实存在的性能杀手**：同一缓存行上的两个热点字段会造成缓存行在核心间弹跳，用 `_ [56]byte` 之类的填充可以带来数倍提升，但只应对真正的热点字段使用。
- **避免过度并发**：批处理、控制并发度、缩小临界区，通常比增加 goroutine 更有效；`schedtrace` 的 `spinningthreads`、`trace` 的调度延迟 profile 都是"你已经并发过头了"的信号。

## 11.9 练习题

### 基础题（巩固语法与 API）

1. **（考察 Context 的取消判定）** 写一个函数 `WaitOrCancel(ctx context.Context, d time.Duration) error`：等待 `d` 时长后返回 nil，若期间 ctx 被取消则提前返回错误。要求在调用方分别用 `WithCancel` 和 `WithTimeout` 触发，并用 `errors.Is` 打印出两种不同的取消原因。请说明为什么不能直接用 `ctx.Err() == context.Canceled` 判断。

2. **（考察 `defer cancel()` 与泄漏）** 写出下面这段代码的问题，并给出修复版本：一个循环里每次迭代都调用 `context.WithTimeout(context.Background(), time.Second)` 并把它传给一个 HTTP 请求，但从未调用返回的 cancel。请说明泄漏的到底是什么资源，以及为什么把父 Context 换成函数参数的 `ctx` 也不能免除这个责任。

3. **（考察 `WithValue` 的规范）** 实现一对函数 `WithRequestID(ctx, id) ctx` 与 `RequestID(ctx) string`，要求：键类型不会被外部包构造、取值时不会 panic、缺失时返回空字符串。然后回答：为什么不能用内置 `string` 作为键？如果两个不同的包都用了同名自定义类型（都叫 `ctxKey`）会发生什么？

4. **（考察 channel 的所有权与关闭规则）** 实现一个 `merge(ctx context.Context, chans ...<-chan int) <-chan int`，把任意多个输入 channel 合并成一个输出。要求：输出必须在所有输入结束后关闭、ctx 取消时立即返回、关闭动作发生在发送方一侧。写完后请用 `runtime.NumGoroutine()` 验证提前返回不会泄漏 goroutine。

### 进阶题（理解原理与实现机制）

5. **（考察 GMP 与 work stealing）** 用 `GODEBUG=schedtrace=1000` 观察一个"8 个 goroutine 抢 4 个 P"的纯 CPU 程序，逐字段解释输出的含义。然后改造成"其中 1 个 goroutine 频繁做阻塞系统调用（例如写文件）"，观察 `threads`、`idlethreads`、`runqueue` 的变化，并解释这些变化与 hand off 机制的关系。

6. **（考察抢占式调度）** 分别写两个死循环：一个是纯算术自增 `for { x++ }`，另一个是每轮都调用一个空函数 `for { runtime.Gosched(); f() }` 之外的、带函数调用的循环。用 `GOMAXPROCS=1` 运行并观察其他 goroutine 是否能得到调度、GC 是否还会正常触发，解释 Go 1.14 前后这两段代码行为的差异。

7. **（考察 happens-before 与竞态检测）** 给定一个"生产者写 slice、消费者读 slice，用裸 `bool` 标志位同步"的程序，先用 `go run -race` 复现竞态报告，再用三种方式修复（channel、`atomic.Bool`、mutex），并用基准测试比较三种修复方式的吞吐差异。请明确指出每种修复方式依赖的是内存模型里的哪一条规则。

8. **（考察 errgroup 的语义）** 写一个包含 10 个子任务的 `errgroup` 程序：其中第 3 个任务必定失败，其余任务会跑 100 毫秒。分别试验（a）不使用 `WithContext`、（b）使用 `WithContext` 但子任务不监听 ctx、（c）使用 `WithContext` 且子任务监听 ctx 三种情况，记录程序总耗时，并解释为什么只有第三种能在第一个错误发生时提前退出。

### 挑战题（综合运用）

9. **（考察 Worker Pool + 错误收集 + 优雅退出）** 实现一个服务：从一个 channel 接收任务、用可控并发度（5~20 个 worker，队列满则扩容、空闲则缩容）处理、把每个任务的错误汇总后以 `errors.Join` 返回，并在收到 `SIGINT`/`SIGTERM` 时停止接收新任务、等待在途任务完成（最多 5 秒）、最后打印统计报告。要求：不存在泄漏的 goroutine、所有 channel 由发送方关闭、有完整的单元测试并跑通 `go test -race`。

10. **（考察 Pipeline + 限流 + 重试）** 构建一条三级流水线：从文件读取 URL 列表 → 用令牌桶限制为 20 QPS 并发抓取（并发度 8）→ 对失败的抓取做指数退避重试（最多 3 次，含抖动，受整体超时约束）→ 统计成功/失败/重试次数。要求中途 `cancel()` 能在一秒内让所有阶段退出，并解释为什么"限流器"和"重试器"必须放在同一层而不是各放一层。

11. **（考察缓存击穿与 singleflight）** 实现一个带 TTL 的本地缓存，要求：并发访问同一个过期 key 时只回源一次（用 `singleflight`）；回源失败时不污染缓存；支持主动失效（`Forget` + 删缓存）。请分别测量"直接加锁"、"缓存 + singleflight"两种实现在 1000 并发下的回源次数与 p99 延迟，并说明 `singleflight` 在多实例部署下的局限与替代方案。

12. **（考察内存模型 + 无锁编程）** 用 `sync/atomic` 实现一个无锁的单生产者单消费者环形队列（固定容量、`head`/`tail` 两个原子变量、满/空判定），要求：不使用任何 mutex、通过 `go test -race`、能在 4 个核上正确运行。然后回答三个问题：（a）它的安全性依赖内存模型里的哪一条规则？（b）把容量设为 2 的幂并用位运算取模有什么好处？（c）如果改成多生产者多消费者，需要引入什么（提示：CAS 循环与 ABA 问题）？请说明为什么此时"分片 + mutex"往往比纯无锁实现更划算。

