# 第 10 章 并发编程基础：Goroutine、Channel 与同步原语

> 本章导读：并发是 Go 语言最鲜明的标签，也是初学者最容易"写出能跑但会崩"的领域。本章从并发与并行的概念差异讲起，系统拆解 goroutine 的启动、调度、生命周期与泄漏排查，把 channel 的语法、同步语义与底层结构讲透，再逐一过一遍 `sync` 与 `atomic` 包里的同步原语，最后落到数据竞争的检测方法与常见并发陷阱清单。学完本章，你应当能够独立写出并发安全、能正常退出、可被 `-race` 检出的程序，并具备定位 goroutine 泄漏与死锁的能力。

## 10.1 并发与并行

### 10.1.1 并发是"结构"，并行是"执行"

Rob Pike 在 2012 年的著名演讲《Concurrency is not Parallelism》中给出了两句几乎被引用到烂的定义：

- **并发（Concurrency）** 是一种把程序拆成若干个可以独立执行的部件的**结构化方式**（a way to structure a program by breaking it into pieces that can be executed independently）。
- **并行（Parallelism）** 是**同时执行**多个计算（doing lots of things at once），它关注的是执行层面。

中文语境里最容易混淆的一句话是"并发是同时应对多件事，并行是同时做多件事"。这里的"应对"（dealing with）和"做"（doing）是关键区别：

| 维度 | 并发 | 并行 |
| --- | --- | --- |
| 关注点 | 程序结构 / 任务分解 | 执行效率 / 吞吐能力 |
| 是否依赖多核 | 不依赖，单核分时也能并发 | 依赖，必须有多个物理执行单元 |
| 典型手段 | goroutine、channel、select、epoll | 多核 CPU、SIMD、GPU、多机 |
| 正确性目标 | 逻辑正确、无竞态、可取消 | 加速比、扩展性 |
| 出错的后果 | 死锁、数据竞争、泄漏 | 加速比不升反降（伪共享、锁竞争） |

一个只看并发、不看并行的例子：单核机器上跑一万个 goroutine 处理 HTTP 请求，程序是并发的（结构上同时应对一万个连接），但任一时刻只有一个 goroutine 在 CPU 上真正执行，谈不上并行。反过来，一段用 `for` 循环做矩阵乘法的代码即使跑在 64 核机器上，没有并发结构也无法自动并行——并行是"加出来的"，不是"送出来的"。

🔥 **重点**：并发结构做好之后，并行只是"多给几个 P"的问题；并发结构没做好，加核数只会让 bug 出现得更频繁。这也是为什么本章先花力气讲同步与所有权，而不是先去谈性能调优。

### 10.1.2 进程、线程、协程与 goroutine

要理解 goroutine 为什么"便宜"，必须把它和进程、内核线程放在同一张表里比较。下表的数字都取数量级与典型值，具体机器上会有差异：

| 维度 | 进程 | 内核线程 | 协程（通用概念） | goroutine |
| --- | --- | --- | --- | --- |
| 地址空间 | 独立，切换需换页表 | 共享进程地址空间 | 共享 | 共享 |
| 初始栈大小 | 由内核决定（用户栈通常 8 MB 虚拟地址） | 典型 1~8 MB（Linux 默认 8 MB 虚拟） | 由语言/库决定，常为几 KB | 2 KB（`runtime` 中 `stackMin = 2048`） |
| 栈能否增长 | 固定或受 `rlimit` 限制 | 固定 | 视实现而定 | 可以，按需翻倍复制，64 位上限 1 GB |
| 创建开销 | 高（内核对象、页表） | 约数微秒到数十微秒 | 低 | 约几百纳秒量级，仅需一个约 2 KB 的栈与一个 `g` 结构体 |
| 切换成本 | 高（含 TLB / 页表开销） | 约 1~2 微秒（含陷入内核） | 低 | 约百纳秒量级（用户态保存少量寄存器） |
| 调度主体 | 操作系统内核 | 操作系统内核 | 语言运行时或库 | Go 运行时（用户态调度器） |
| 数量级 | 几十到几百 | 几百到几千 | 可达百万（内存允许） | 十万级很常见，百万级可行 |
| 通信方式 | IPC、共享内存、socket | 共享内存 + 同步原语 | channel / 共享变量 | channel 优先，也可共享内存 |

几个必须澄清的细节：

1. **"初始 2 KB"并不是让你随便开一百万个。** 每个 goroutine 还有 `g` 结构体、栈的元数据、调度队列条目等额外开销，实测量级是每个 goroutine 数 KB。一百万 goroutine 大致对应数百 MB 到 GB 级内存，取决于栈增长情况。
2. **栈的 1 GB 上限是"每 goroutine"的默认上限。** 源码中 `maxstacksize = 1000000000`（64 位平台），32 位平台为 250 MB。还有一种"极端上限" `maxstackceiling = 2 * maxstacksize`，只有通过 `debug.SetMaxStack` 调整时才会碰到。栈超过 1 GB 时运行时会打印 `runtime: goroutine stack exceeds 1000000000-byte limit` 并直接崩溃——通常说明写了无限递归。
3. **goroutine 的栈是可搬移的。** 栈不够时运行时分配一块更大的内存并整体复制过去，栈上所有指针都要被正确调整。这带来一个重要推论：Go 的栈地址不稳定，因此把 Go 指针长期交给 C 代码保存是危险的（cgo 有严格的指针传递规则）。这也是 Go 不能像某些语言那样简单地把"goroutine 当线程"的原因之一。
4. **协程不等于 goroutine。** Python 的 `async/await`、Kotlin 的协程大多是**协作式**的：你不 `await` 或 `yield`，别人就没机会运行。Go 的调度器在 Go 1.14 之后是**基于信号的真抢占式**：长时间运行、没有函数调用、没有 channel 操作的纯计算循环也会被 `sysmon` 抢走 P，不会"一个 goroutine 卡死整个程序"。

### 10.1.3 Go 的并发哲学：通过通信来共享内存

Go 官方文档《Effective Go》里有一句话，几乎每个 Go 程序员都能背下来：

> Do not communicate by sharing memory; instead, share memory by communicating.
> （不要通过共享内存来通信，而要通过通信来共享内存。）

**这句话到底在说什么。** 传统多线程编程的默认流程是：多个线程都持有同一个变量的指针 → 各自读改写 → 靠互斥锁把临界区围起来 → 正确性依赖于"每个程序员在每一处都记得加锁"。一旦有人漏了一次加锁，你就得到了一个数据竞争（Data Race），而数据竞争的后果是不可预测的（详见 10.6）。

"通过通信共享内存"则换了一个思路：**数据在同一时刻只属于一个 goroutine**。当你想把数据交给别人处理时，把数据本身（或它的指针）通过 channel 发送过去，发送方从此不再碰它，接收方成为唯一持有者。所有权（Ownership）在一瞬间完成转移，不需要锁，因为根本没有"同时访问"。这就是 Rust 用类型系统表达的 `Send` 语义，Go 用 channel 表达它，只是把检查交给了程序员和 `-race`。

**这句话不是说 mutex 没用。** 这是初学者最常有的误读。几个反证：

- `sync` 包是标准库的一等公民，`sync.Pool`、`sync.Map`、`sync.WaitGroup` 被标准库和主流框架大量使用；`runtime` 内部更是到处是自旋锁与 `mutex`。
- channel 的底层实现 `hchan` 里就有一把锁（`lock mutex`），"用 channel 就不用锁"是一种误解——你只是把锁的使用封装到了运行时里。
- 有些状态天然是"共享"而不是"流动"的：一个进程级的配置缓存、一个引用计数、一个连接池的统计数字。为了这些状态去构造一个 channel 加收发协程，往往是自找麻烦。
- 有大量真实场景用 mutex 更清晰、更快：保护一小段临界区（几个字段的读改写）时，`Mutex` 的开销通常在几十纳秒量级，而一次带调度的 channel 往返要贵得多。

**选择标准可以概括成一句话：数据是"流动的"还是"共享的"。**

| 场景特征 | 推荐手段 |
| --- | --- |
| 任务分发、结果收集、流水线阶段之间传递数据 | channel |
| 事件循环、多路等待、超时与取消 | channel + select |
| 生命周期编排（谁等谁结束） | WaitGroup、channel、`context` |
| 保护一小段临界区的共享状态（计数器、缓存、配置） | Mutex / RWMutex / atomic |
| 单次初始化、对象复用 | sync.Once、sync.Pool |
| 高频读、极少写的只读快照 | atomic.Pointer[T]、RWMutex |

### 10.1.4 CSP 模型与 Go 的实现取舍

channel 的思想来源是 C.A.R. Hoare 于 1978 年发表的论文《Communicating Sequential Processes》（通信顺序进程，CSP）。原版 CSP 是一门**进程代数**式的形式化语言：进程之间通过**具名**的、**同步**的通信原语交换消息，并且提供 `□`（选择）、`||`（并行组合）等组合子来描述程序结构。

Go 只借鉴了其中的核心部分，并且做了几处明确的取舍。Rob Pike 本人说过，Go 的并发**不是** CSP，而是 CSP 与传统共享内存多线程的融合（a fusion）。

| CSP 原始模型 | Go 的实现 |
| --- | --- |
| 进程是匿名的、具名的通信端点 | channel 是第一类值，可以传递、可以放进结构体、可以作为参数 |
| 通信按进程名配对 | 通信按 channel 配对，channel 有类型 |
| 通信是同步的（会合） | 无缓冲 channel 是同步的；有缓冲 channel 提供异步性 |
| 选择算子是 `□`（守卫命令） | `select` 语句，且支持 `default` 与随机选择 |
| 无共享内存 | 提供 `sync`、`atomic`，与 channel 并存 |
| 进程树有严格结构 | goroutine 之间无父子关系，取消靠 `context` 显式传播 |

最后一条尤其重要：Go 里 goroutine 是"扁平"的，运行时不知道谁创建了谁（除了调试栈信息）。这带来两个后果：

- **好处**：创建、销毁、迁移都极轻，调度器不必维护父子树。
- **代价**：**取消（Cancellation）必须显式传播**。父 goroutine 退出时，子 goroutine 不会自动结束，于是就有了 10.2.6 要详细讲的 goroutine 泄漏。标准库给出的解法是 `context.Context`（第 11 章详述），它本质上是"一棵由 channel 组成的树"。

## 10.2 Goroutine

### 10.2.1 `go` 关键字：语法与求值时机

启动一个 goroutine 的语法只有一条：

```text
go 函数调用
```

它要求后面必须是一个**函数或方法调用**，不能是任意语句，也不能接收返回值。三种常见写法：

```go
// 片段：省略 package main 与 import，仅示意关键代码（含 ... 占位，不能直接编译）
go doWork()            // 普通函数
go s.handle(req)       // 方法调用
go func() { ... }()    // 立即调用的匿名函数（闭包）
```

这里有一个几乎人人都踩过的细节：**函数值与实参在当前 goroutine 中"立即求值"，函数体才在新 goroutine 中执行**。也就是说 `go f(x)` 的含义是"先算好 `f` 和 `x`，再另起一个 goroutine 执行 `f(x)`"。

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		// 形参 n 在启动 goroutine 之前就完成了求值与拷贝，
		// 所以每个 goroutine 拿到的是自己那一份 i 的快照。
		go func(n int) {
			defer wg.Done()
			fmt.Printf("worker %d 收到 %d\n", n, n)
		}(i)
	}

	wg.Wait()
	fmt.Println("全部完成")
}
```

**解读**：这是 Go 1.22 之前"循环变量捕获"问题的经典解法——**把循环变量当实参传进去**。Go 1.22 修改了 `for` 循环的语义，`for i := 1; ...` 与 `for i := range` 中的循环变量改为**每次迭代一个新变量**，所以即使写成 `go func() { fmt.Println(i) }()` 也不会再捕获到同一个变量（前提是模块的 `go` 指令 ≥ 1.22，细节见 10.9）。不过"显式传参"依然是更好的风格：它让变量的归属一目了然，也不受 `go.mod` 中语言版本的影响。

另一个求值时机的坑在于**共享变量的读取被推迟了**：

```go
// 片段：省略 package main 与 import，仅示意关键代码
n := 0
go func() {
	fmt.Println(n) // 读取发生在未来某个时刻，那时 n 可能已经变了
}()
n = 100
```

这段代码的输出是 `0` 还是 `100`，取决于两个 goroutine 的调度顺序——它同时是一个数据竞争。要消除不确定性，就在 `go` 之前把值算好：`n := 0; go func(v int) { ... }(n)`。

⚠️ **注意**：`go` 语句**不能带返回值**。`x := go f()` 是语法错误。要从 goroutine 拿结果，必须让它写进 channel（或写进由外部同步保护的变量）。

💡 **提示**：goroutine 中未捕获的 panic 会**终止整个进程**，而不是只终止那一个 goroutine（运行时打印 `panic: ... [recovered]` 之类的栈后 `exit(2)`）。所以每个长期运行的 goroutine 都应该在自己的入口处 `defer` 一个 `recover`，把 panic 转成日志或错误值。这一点与 Java 的线程池"一个任务崩了不影响别人"完全不同。

### 10.2.2 goroutine 的栈与用户态调度

goroutine 之所以能做到"几十万个也不怕"，靠的是两件事：**小栈 + 可增长**，以及**用户态调度**。

**栈：小、可增长、可搬移。** 新 goroutine 的栈初始为 2 KB（源码常量 `stackMin = 2048`，在 Windows 上还会额外加一个 4 KB 的 `stackSystem` 用于保护区）。编译期编译器会在每个函数的序言（prologue）里插入栈边界检查，函数需要的栈帧一旦逼近剩余空间，就调用 `runtime.morestack` 触发扩容。扩容策略是**加倍**：2 KB → 4 KB → 8 KB …… 上限 1 GB。因为栈是连续内存，扩容时会分配新的更大栈并**整体复制**，然后修正所有指向旧栈的指针——这个动作叫栈复制（stack copy），所以 goroutine 的栈地址会变。

**调度：GMP 三件套。** Go 运行时的调度器由三种实体组成：

- **G**（goroutine）：要执行的任务，包含栈、状态、`gobuf`（保存的寄存器上下文）。
- **M**（machine）：内核线程，真正干活的人。M 必须绑定一个 P 才能执行 Go 代码。
- **P**（processor）：逻辑处理器，数量等于 `GOMAXPROCS`。P 持有本地可运行队列（runq，容量 256）。

调度的大致流程：新建的 goroutine 优先挂在当前 P 的本地队列尾部；本地队列满了就把一半搬到全局队列；P 空闲时会先从全局队列取一批，如果还是没有，就去**偷**别的 P 的队列（work stealing，一次偷一半）。当某个 goroutine 发起阻塞的系统调用时，运行时会把它所在的 M 与 P **解绑**（handoff），把 P 交给另一个 M，避免一个阻塞调用浪费一个逻辑处理器。此外，`sysmon` 后台线程负责抢占运行过久的 goroutine、回收长时间处于系统调用中的 P。

🔥 **重点**：`GOMAXPROCS` 限制的是**同时执行 Go 代码的 P 的数量**，不是线程数，也不是 goroutine 数。一个程序里 M 的数量可以远大于 `GOMAXPROCS`（因为大量阻塞的系统调用会创建新线程）。

### 10.2.3 main goroutine 与程序退出

`main` 函数本身运行在一个 goroutine 里，通常称为 **main goroutine**。它的特殊之处只有一条，但极其重要：

> **当 `main` 函数返回时，程序立即结束。所有其他正在运行的 goroutine 会被直接杀死，不会等待，也不会执行它们的 `defer`。**

这不是"优雅退出"，这是 `runtime` 直接调用 `exit(0)`。写并发程序时，绝大多数"代码明明写了却没输出"的问题都源于此。

下面这个程序就是最典型的翻车现场：

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	go func() {
		time.Sleep(50 * time.Millisecond)
		fmt.Println("worker: 我干完了") // 这一行永远不会被打印
	}()

	time.Sleep(10 * time.Millisecond)
	fmt.Println("main: 我要退出了")
}
```

运行结果：

```text
main: 我要退出了
```

**现象**：worker 里的 `fmt.Println` 从来没有执行到，程序也没有报错、没有 panic，一切"看起来正常"。这是最危险的一类 bug：**静默丢数据**。

**修复方式一：用 `sync.WaitGroup` 等待。**

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		fmt.Println("worker: 我干完了")
	}()

	wg.Wait() // 阻塞到计数器归零
	fmt.Println("main: 现在可以安全退出了")
}
```

**修复方式二：用 channel 收一个完成信号。** 适合"只有一个结果要拿"的场景：

```go
package main

import "fmt"

func main() {
	done := make(chan struct{})

	go func() {
		defer close(done) // close 广播"我结束了"
		fmt.Println("worker: 干活中")
	}()

	<-done
	fmt.Println("main: 收到完成信号")
}
```

**解读**：`chan struct{}` 是 Go 中表达"纯信号"的惯用法——`struct{}` 占 0 字节，channel 只承担同步作用，不传递数据。用 `<-done` 接收而不是 `for range`，是因为我们只关心"至少完成一次"。

⚠️ **注意**：不要用 `time.Sleep` 代替同步。`time.Sleep(1 * time.Second)` 在开发机上"总是能跑通"，到了负载高的 CI 或生产环境就会随机失败——这类测试被称为 flaky test，是最消耗团队信任的 bug 类型。**只在演示"错误示范"时使用 Sleep，真实代码里一律用同步原语。**

### 10.2.4 `sync.WaitGroup` 的正确用法

`WaitGroup` 是"等待一组 goroutine 结束"的标准工具，三个方法：

- `Add(delta int)`：把内部计数器加 `delta`（可正可负，通常为正）。
- `Done()`：等价于 `Add(-1)`。
- `Wait()`：阻塞直到计数器归零。

它的正确用法有四条铁律，每一条背后都对应一个真实的线上事故：

**铁律一：`Add` 必须在启动 goroutine 之前调用。** 反例是把 `wg.Add(1)` 写进 goroutine 内部：

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄（含 ... 占位，不能直接编译）
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
	go func() {
		wg.Add(1) // ❌ 危险：Wait 可能在 Add 之前就返回了
		defer wg.Done()
		// ... 干活
	}()
}
wg.Wait()
```

**现象**：`Wait()` 有可能在任何一个 `Add` 之前执行，此时计数器为 0，`Wait` 立刻返回，主程序退出，**子 goroutine 全部被杀**。更糟的情况是 `Wait` 已经观察到计数器为 0 并返回，随后某个 `Add` 才执行，语义完全错乱；如果 `Add` 与 `Wait` 并发发生，"计数器归零后再 Add"本身就是一个竞态。正确做法是把 `wg.Add(1)` 放在 `go` 语句**之前**，让"计数增加"与"启动任务"在同一个 goroutine 中顺序发生。

**铁律二：`defer wg.Done()` 要放在 goroutine 的第一行。** 这样即使函数体中间 panic（并已被 `recover`），计数器也会递减；如果写成逻辑末尾的 `wg.Done()`，一旦中途 `return` 或 panic，`Wait()` 就会永久阻塞。

**铁律三：不要拷贝 `WaitGroup`。** `WaitGroup` 内部含 `noCopy` 字段和原子状态，拷贝之后是两个独立的计数器，`Done` 减的是副本，原件的计数永远归不了零。

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄
func bad(wg sync.WaitGroup) { // ❌ 值传递：拷贝了一份 WaitGroup
	defer wg.Done()
	return
}

func good(wg *sync.WaitGroup) { // ✅ 传指针
	defer wg.Done()
	return
}
```

**现象**：`bad` 版本中 `wg.Done()` 作用在副本上，调用方的 `Wait()` 会永久阻塞（如果还有别的 goroutine 在等，运行时最终会报 `fatal error: all goroutines are asleep - deadlock!`）。好消息是 `go vet` 的 `copylocks` 检查会直接报错：`passes lock by value: sync.WaitGroup contains sync.noCopy`。**养成提交前跑 `go vet ./...` 的习惯，这类错误根本到不了运行时。**

**铁律四：复用前必须等计数器归零。** `WaitGroup` 可以重复使用，但必须等上一轮 `Wait()` 返回之后再做下一轮 `Add`。模式是"每轮 `Wait` 结束后计数器归零，然后开始新一轮"。绝不允许"上一轮还没结束就复用同一个 `WaitGroup` 启动下一轮任务"，否则两轮的计数会混在一起，`Wait` 等到的边界完全不正确。

**Go 1.25 新增的便捷方法：`WaitGroup.Go`。** 它把"`Add(1)` + 启动 goroutine + `defer Done()`"三步合成一步，从 API 层面消除了"忘了 Add"和"Add 位置不对"的问题：

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0

	for i := 1; i <= 5; i++ {
		wg.Go(func() { // 内部自动 Add(1)，函数返回时自动 Done()
			mu.Lock()
			total += i
			mu.Unlock()
		})
	}

	wg.Wait()
	fmt.Println("total =", total) // total = 15
}
```

**解读**：`wg.Go(f)` 内部等价于 `wg.Add(1); go func() { defer wg.Done(); f() }()`。注意上面的闭包捕获了循环变量 `i`——在 Go 1.22 及以后，每次迭代的 `i` 是独立变量，结果是确定的 15；如果模块的 `go` 指令低于 1.22，这段代码会产生数据竞争与错误的和。`total` 的累加则用 `Mutex` 保护，因为这是多个 goroutine 对**同一个**变量的写操作（即"共享内存"场景）。

### 10.2.5 `Gosched`、`NumGoroutine` 与 `GOMAXPROCS`

**`runtime.Gosched()`**：让当前 goroutine 主动放弃 CPU，把自己放回可运行队列尾部，让调度器去跑别的 goroutine。它**不会**阻塞、不会释放锁、只是"让一下"。绝大多数情况下你不需要它——Go 1.14 之后的抢占式调度已经能处理 CPU 密集的循环。它的合理用途是：在自旋等待（spin-wait）循环里避免长时间霸占 P，以及在教学演示中人为制造 goroutine 交错。

**`runtime.NumGoroutine()`**：返回当前存在的 goroutine 数量（包含正在运行和已阻塞的）。这是一个**排障指标**：如果它在程序空闲时持续增长，几乎可以断定存在 goroutine 泄漏。

**`GOMAXPROCS` 的含义与现代默认值**：

- `runtime.GOMAXPROCS(n)`：设置可同时执行 Go 代码的逻辑处理器数量，返回**旧值**；`n <= 0` 表示只查询不修改（所以 `runtime.GOMAXPROCS(0)` 是"读取当前值"的标准写法）。
- **默认值的历史**：Go 1.5 之前默认是 1，Go 1.5 起默认等于 `runtime.NumCPU()`，Go 1.25 起在 Linux 上还会考虑容器的 CPU 配额（cgroup quota）与进程的 CPU 亲和性掩码，取三者的最小值，并**每秒自动更新**（最多每秒一次）。运行时内部还会保证：除非逻辑 CPU 数或亲和性掩码本身就小于 2，否则不会把 `GOMAXPROCS` 设为小于 2。
- 一旦你用环境变量 `GOMAXPROCS` 或调用 `runtime.GOMAXPROCS(n)` 显式设置了值，**自动更新就停止**；想恢复到默认行为可以调用 `runtime.SetDefaultGOMAXPROCS()`。
- 调试开关：`GODEBUG=containermaxprocs=0,updatemaxprocs=0` 会关闭上述容器感知与自动更新行为。

```go
package main

import (
	"fmt"
	"runtime"
	"sync"
)

func main() {
	fmt.Println("逻辑 CPU 数：", runtime.NumCPU())
	fmt.Println("GOMAXPROCS：", runtime.GOMAXPROCS(0))

	old := runtime.GOMAXPROCS(2) // 返回值是旧值
	fmt.Printf("显式设置为 2（旧值 %d），显式设置后自动更新停止\n", old)

	var wg sync.WaitGroup
	start := runtime.NumGoroutine()
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			runtime.Gosched() // 主动让出，观察调度器的交错
		})
	}
	wg.Wait()
	fmt.Printf("goroutine 数：启动前 %d，结束后 %d\n", start, runtime.NumGoroutine())
}
```

**解读**：`runtime.GOMAXPROCS(2)` 的返回值是**修改前**的值，因此要读当前值请传 0。把 `GOMAXPROCS` 设为 1 是一种经典的**竞态复现手段**：单 P 之下 goroutine 的交错更规律，很多在 8 核机器上"偶尔出现"的 bug 会稳定复现。反之，把 `GOMAXPROCS` 调大并不能修复竞态——它只会让 bug 更难复现。

💡 **提示**：`GOMAXPROCS` 不是"越多越快"。当 goroutine 数量远大于核数、且临界区竞争激烈时，P 越多锁竞争与 cache 跳动越严重，吞吐反而可能下降。判断依据永远是压测数据，而不是直觉。

### 10.2.6 goroutine 泄漏：定义、成因与排查

**定义**：goroutine 泄漏是指某个 goroutine **永久地阻塞在某个永远不会发生的条件上**，再也不会继续执行，但它占用的栈、`g` 结构体以及它引用的整条对象图（包括 channel、切片、缓冲区里的数据）都无法被回收。泄漏的 goroutine 不会被 GC 清理——只要它还"活着"（哪怕在阻塞），运行时就必须保留它的上下文。

**三种典型成因。** 它们的共同点是"有一方在等一件永远不会发生的事"：

| 成因 | 代码形态 | 谁在等 | 为什么永不发生 |
| --- | --- | --- | --- |
| 发送端阻塞、无人接收 | `go func() { ch <- v }()` 之后函数直接返回 | 发送方 | channel 无缓冲或已满，且接收方已经走了 |
| 循环无退出条件 | `for { select { case v := <-ch: ... } }` 没有 `case <-ctx.Done()` | 接收方 | 上游不再发送，但 goroutine 不退 |
| 等待永远不会关闭的信号 | `<-done`，而 `done` 只在错误分支里被关闭 | 接收方 | 正常路径忘记 `close(done)` |

还有两个隐蔽的变体：**接收方等一个永远不会被关闭的 channel**（`for v := range ch` 依赖 `close` 才会退出），以及**向 nil channel 发送/接收**（永久阻塞，见 10.3.5——它在 `select` 里是技巧，在普通语句里是灾难）。

下面这段程序故意泄漏 5 个 goroutine，并用两种手段把它揪出来：

```go
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

// leaky 每次调用都会泄漏一个 goroutine：发送方永远等不到接收方。
func leaky() {
	ch := make(chan int)
	go func() {
		ch <- 42 // 无缓冲 channel，没有接收者 → 永久阻塞
	}()
	// 函数返回，ch 对调用者已不可达，但阻塞中的 goroutine 仍被运行时引用
}

func main() {
	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		leaky()
	}
	time.Sleep(50 * time.Millisecond) // 等这些 goroutine 真正阻塞下来
	fmt.Printf("goroutine 数：%d → %d\n", before, runtime.NumGoroutine())

	// 打印所有 goroutine 的调用栈，定位泄漏点
	pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
}
```

**输出（节选）**：

```text
goroutine 数：1 → 6
goroutine profile: total 6
5 @ 0x7ff6a55595ae 0x7ff6a54f12fc 0x7ff6a54f0ef7 0x7ff6a55bd69e 0x7ff6a555fcc1
#	0x7ff6a55bd69d	main.leaky.func1+0x1d	D:/demo/main.go:15
```

（地址每次运行都不同，行号与函数名才是关键信息。）

**解读**：`total 6` 说明泄漏已经发生；`5 @ ...` 表示有 5 个 goroutine 卡在**同一个**栈位置；最后一行直接指出罪魁祸首是 `main.leaky.func1` 的第 15 行——也就是 `ch <- 42`。这就是 goroutine 泄漏排查的标准流程：

1. **发现**：`runtime.NumGoroutine()` 周期性上报，或在监控里看进程 RSS 与 goroutine 数只涨不跌。线上程序更常用 `net/http/pprof` 暴露 `/debug/pprof/goroutine?debug=1`，用 `go tool pprof` 抓取并对比时间点。
2. **定位**：按调用栈的 `total` 排序，找到数量最多、且**不是** `runtime.gopark` 里那些正常阻塞（如网络轮询、`time.Sleep`）的那一组栈。
3. **修复**：修改代码，让每一方都能退出。

**修复模式一：`select` + `ctx.Done()`（最通用）。** 给 goroutine 一个"取消信号"，任何阻塞点都同时监听它：

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// worker 在取消或超时后一定会退出，不会泄漏。
func worker(ctx context.Context, ch <-chan int) {
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				fmt.Println("worker: 上游已关闭，正常退出")
				return
			}
			fmt.Println("worker: 处理", v)
		case <-ctx.Done():
			fmt.Println("worker: 收到取消信号，退出：", ctx.Err())
			return
		}
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel() // 无论正常返回还是提前退出，都要释放定时器

	ch := make(chan int, 1)
	ch <- 1
	go worker(ctx, ch)

	time.Sleep(120 * time.Millisecond) // 仅为演示：等 worker 因超时退出
	fmt.Println("main: 结束")
}
```

**修复模式二：把发送变成"可以放弃的"。** 当 goroutine 可能没人接收时，用 `select` + `default` 或 `ctx.Done()` 让发送可被放弃：

```go
// 片段：省略 package main 与 import，仅示意关键代码
select {
case ch <- v:
	// 发送成功
case <-ctx.Done():
	return
}
```

**修复模式三：发送方负责 `close`，接收方用 `for range` 自然退出。** 这是"生产者-消费者"的标准收尾方式，也是最容易被忘记的一半——只有 `close` 了，`range` 才会结束（见 10.3.4）。

⚠️ **注意**：**不要在循环里反复 `select { case <-time.After(d): }` 并指望它便宜。** 在 Go 1.23 之前，`time.After` 创建的定时器在触发前不会被回收，短周期循环会积累大量定时器（既是内存泄漏，也是"定时器泄漏"）。Go 1.23 起，未被引用的 `Timer`（以及 `time.After` 创建的定时器）可以被垃圾回收，这个坑已经大大缓解；但在热循环里仍然推荐用 `time.NewTimer` 配合 `Reset`，或者直接用 `context.WithTimeout`。

## 10.3 Channel 基础

如果说 goroutine 是 Go 并发的"执行单元"，channel 就是它的"连接线"。channel 是一个**带类型的、并发安全的 FIFO 通道**，它同时承担三种职责：传递数据、同步执行、广播事件。

### 10.3.1 声明与创建

channel 是引用类型，`chan T` 是它的类型字面量（T 是元素类型）。有且仅有三种"出身"：

```go
// 片段：省略 package main 与 import，仅示意关键代码
var ch chan int          // 声明一个 nil channel（零值）
ch1 := make(chan int)    // 无缓冲 channel（unbuffered），cap 为 0
ch2 := make(chan int, 8) // 有缓冲 channel（buffered），cap 为 8
```

长度与容量的语义与切片完全不同，必须记准：

| 表达式 | 含义 |
| --- | --- |
| `len(ch)` | 当前**缓冲区中排队等待被接收**的元素个数 |
| `cap(ch)` | 缓冲区容量；无缓冲为 0；nil channel 上两者都为 0 |

`var ch chan int` 得到的是一个 **nil channel**（零值）。它与"已经 `make` 但缓冲区空"的 channel 完全不同：nil channel 上的读写会**永久阻塞**。这一点在 10.3.5 有专门讨论，因为它在 `select` 里是神器，在普通语句里是灾难。

⚠️ **注意**：`make(chan int, -1)` 会 panic（`makechan: size out of range`）。容量必须是常数或运行期非负整数。

### 10.3.2 发送、接收与关闭的语法

三个操作符，一个记忆技巧：

```go
// 片段：省略 package main 与 import，仅示意关键代码
ch <- v     // 发送：箭头的方向是"数据流进 channel"
v := <-ch   // 接收：箭头的方向是"数据从 channel 流出"
close(ch)   // 关闭：告诉所有接收者"不会再有数据了"
```

💡 **记忆技巧**：**把 `chan` 当成一个"管道口"，箭头永远指向数据流动的方向。** `ch <- v` 表示数据从 `v` 流入 `ch`；`<-ch` 表示数据从 `ch` 流出到接收方。所以 `chan<- T` 是"只能往里写"的发送通道，`<-chan T` 是"只能往外读"的接收通道——箭头贴着 `chan` 关键字的那一侧就是**允许的方向**。

一个容易看错的细节：`<-ch` 作为一个表达式整体是"接收操作"，只有在类型位置（如 `chan<- int`）里箭头才是方向标记。

### 10.3.3 无缓冲 channel 的同步语义 vs 有缓冲 channel 的异步语义

这是 channel 最核心、也最常被含糊带过的一点。

**无缓冲 channel 是一次会合（rendezvous）。** 发送方和接收方必须**同时就位**，数据才完成传递；两者只要有一个缺席，另一个就阻塞。因此它不只是"传值"，它还是一个**同步点**：`ch <- v` 返回时，我们可以确定接收方已经拿到值；`v := <-ch` 返回时，我们可以确定发送方已经完成发送。Go 内存模型明确规定这种配对会建立 happens-before 关系——发送方的所有写在接收方看来都是可见的。

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	ch := make(chan string) // 无缓冲
	start := time.Now()

	go func() {
		time.Sleep(30 * time.Millisecond) // 接收方 30ms 后才就位
		fmt.Println("接收方就位，耗时", time.Since(start).Round(time.Millisecond))
		<-ch
	}()

	ch <- "hello" // 发送方会一直阻塞到接收方执行 <-ch
	fmt.Println("发送完成，耗时", time.Since(start).Round(time.Millisecond))
}
```

**现象**：`发送完成` 一定在 `接收方就位` 之后打印，耗时约 30ms。这就是"发送必须等到接收"的直接证据。

**有缓冲 channel 是一次投递。** 只要缓冲区没满，发送方立即返回，不关心有没有人接收；只要缓冲区不空，接收方立即返回，不关心有没有人在发送。缓冲区满时发送阻塞，空时接收阻塞——这就是"背压（backpressure）"的天然实现。

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 3)

	ch <- 1 // 不阻塞
	ch <- 2
	ch <- 3
	fmt.Printf("写入 3 个后：len=%d cap=%d\n", len(ch), cap(ch))

	// ch <- 4 // ❌ 打开这一行：缓冲区已满，主 goroutine 会永久阻塞
	// → fatal error: all goroutines are asleep - deadlock!

	fmt.Println("依次读出：", <-ch, <-ch, <-ch)
	fmt.Printf("读完后：len=%d cap=%d\n", len(ch), cap(ch))
}
```

**解读**：容量为 n 的 channel 允许"发送方最多领先接收方 n 个元素"。

**选缓冲多大的三条经验**：

| 缓冲 | 什么时候用 |
| --- | --- |
| 0（无缓冲） | 需要**严格同步**；需要确认接收方一定收到；事件通知；`close` 广播 |
| n（小且有依据） | 需要**解耦生产与消费的瞬时抖动**，n 取"消费者一次能追上的量"或"生产者一次批次的量" |
| 很大（当队列用） | 通常说明设计错了：把 channel 当成无界队列会掩盖容量问题，内存会随积压无限增长 |

🔥 **重点**：**不要用 `len(ch)` 判断"还有没有数据"来做同步逻辑。** `if len(ch) > 0 { v := <-ch }` 在并发下是典型的 TOCTOU（time-of-check to time-of-use）竞态：检查与取值之间可能有别的 goroutine 把数据取走，也可能有新的数据进来。正确做法是用 `select` + `default` 做非阻塞接收（见 10.4.2）。

### 10.3.4 关闭规则：七条必须背下来的规则

`close(ch)` 的语义是"**不会再有新的值被发送到这个 channel 上**"，它不是"销毁 channel"，也不是"清空缓冲区"。围绕它有一组非常明确的规则：

1. **只能由发送方关闭**。channel 是"生产者-消费者的边界"，关闭是对下游的承诺，只有生产者有资格做出这个承诺。接收方关闭会让发送方在之后的发送中 panic。
2. **向已关闭的 channel 发送会 panic**：`panic: send on closed channel`。
3. **重复关闭会 panic**：`panic: close of closed channel`。
4. **关闭 nil channel 会 panic**：`panic: close of nil channel`。
5. **从已关闭的 channel 接收是安全的**：缓冲区里剩余的数据会被依次读出，读空之后返回**元素类型的零值**且第二个返回值为 `false`。
6. **关闭一个有接收者阻塞的 channel 会唤醒它们**，每个接收者都得到零值 —— 这就是"广播"。
7. **关闭允许接收方安全地判断"是否还有数据"**，这正是 `for range` 能退出的原因。

下面这个程序用 `recover` 把三种 panic 变成可观察的输出，并验证关闭后的接收行为：

```go
package main

import "fmt"

// observe 执行 f，把 panic 转成一行可读的观察结果，便于在同一程序里连续演示多种错误用法
func observe(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%-24s → panic: %v\n", name, r)
		}
	}()
	f()
	fmt.Printf("%-24s → 无异常\n", name)
}

func main() {
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2
	close(ch)

	observe("向已关闭 channel 发送", func() { ch <- 3 })
	observe("重复 close", func() { close(ch) })
	observe("close 一个 nil channel", func() { var nilCh chan int; close(nilCh) })

	// 关闭后的接收：先读完缓冲区里的存量，之后返回零值 + false
	for v := range ch {
		fmt.Println("从已关闭 channel 读到存量：", v)
	}

	v, ok := <-ch
	fmt.Printf("读空之后再接收：v=%d ok=%v\n", v, ok)
	v, ok = <-ch
	fmt.Printf("再接收一次：    v=%d ok=%v（零值可无限重复读出）\n", v, ok)
}
```

**输出**：

```text
向已关闭 channel 发送          → panic: send on closed channel
重复 close                 → panic: close of closed channel
close 一个 nil channel     → panic: close of nil channel
从已关闭 channel 读到存量： 1
从已关闭 channel 读到存量： 2
读空之后再接收：v=0 ok=false
再接收一次：    v=0 ok=false（零值可无限重复读出）
```

**解读**：三点值得注意。第一，三种 panic 的信息是固定的，看到它们就能立刻定位问题。第二，`for range ch` 会把**缓冲区里的存量**全部读完后才退出——`close` 不会丢弃数据。第三，从已关闭 channel 接收**永远不会阻塞**，所以一个"关闭后还在被读"的循环会变成 CPU 空转（见 10.9 的陷阱表）。

**❌ 错误用法：由接收方关闭 channel。**

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄
ch := make(chan int)
go func() {
	v := <-ch
	fmt.Println(v)
	close(ch) // ❌ 接收方关闭
}()
ch <- 42
ch <- 43 // panic: send on closed channel（调度时机不同，可能在第 42 行就炸）
```

**现象**：`panic: send on closed channel`，而且**是否复现取决于调度顺序**——这正是并发 bug 最难排查的地方。**✅ 正确用法**：把 `close` 放在唯一的发送方那里，并且用 `defer` 保证所有返回路径都会关闭：

```go
// 片段：省略 package main 与 import，仅示意关键代码
go func() {
	defer close(ch) // 发送方负责，且只关一次
	for _, v := range data {
		ch <- v
	}
}()
```

**多个发送方怎么办？** 规则仍然是"只能关一次、只有发送方能关"。此时引入一个"收尾 goroutine"：用 `WaitGroup` 等所有发送方退出，再 `close`：

```go
// 片段：省略 package main 与 import，仅示意关键代码
go func() {
	wg.Wait()  // 等所有发送方结束
	close(ch)  // 此时没有发送方了，关闭安全
}()
```

### 10.3.5 nil channel：读写都永久阻塞

nil channel 的行为可以用一句话概括：**在 nil channel 上的任何发送或接收都会永久阻塞**（运行时把它对应的 `sudog` 挂进队列，但没有任何人能唤醒它，因为它连缓冲区都没有）。`len`、`cap` 返回 0，`close` 会 panic。

这看起来是个纯粹的坑，但它是 `select` 里一个极其有用的技巧：**`select` 会忽略那些操作数为 nil channel 的分支**（因为对该 case 求值时，nil channel 上的操作永远不就绪）。于是"动态禁用某个分支"就等价于"把那个 channel 变量置为 nil"：

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	in1 := make(chan string, 1)
	in2 := make(chan string, 1)
	in1 <- "来自 in1"
	in2 <- "来自 in2"

	var enable2 chan string = in2 // 置为 nil 即禁用该分支
	deadline := time.After(200 * time.Millisecond)

	count := 0
	for count < 3 {
		select {
		case v := <-in1:
			fmt.Println("收到", v, "→ 禁用 in1 分支")
			in1 = nil // 用完就禁用，避免重复读取空 channel 造成的空转
		case v := <-enable2:
			fmt.Println("收到", v, "→ 禁用 in2 分支")
			enable2 = nil
		case <-deadline:
			fmt.Println("超时兜底，退出循环")
			count = 3
		default:
			// 三个分支都不就绪时的短暂让路，避免忙等
			time.Sleep(10 * time.Millisecond)
		}
		count++
	}
	fmt.Println("main 结束")
}
```

**解读**：这个模式在处理"多个数据源，每个源只消费一次"时非常常见。注意我用了 `time.After` 做兜底，**保证循环一定能结束**——真实代码里更推荐用 `context.WithTimeout` 或给消费次数设上限，任何 `for { select {...} }` 都必须有明确退出路径。

**❌ 错误用法：把 nil channel 当"空 channel"用。**

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄
var ch chan int
ch <- 1        // ❌ 永久阻塞
// 或
v := <-ch      // ❌ 永久阻塞
```

**现象**：如果这是主 goroutine 唯一的活动，运行时会检测到并报 `fatal error: all goroutines are asleep - deadlock!`；如果在其他 goroutine 里，就是一次静默的 goroutine 泄漏。**✅ 正确用法**：解释清楚"nil 表示禁用"，并且在 `select` 中才使用它；普通流程中一律先 `make`。

### 10.3.6 接收的四种形式

| 形式 | 语义 | 适用场景 |
| --- | --- | --- |
| `v := <-ch` | 接收一个值，**忽略是否已关闭** | 确定还有数据，或不在乎零值 |
| `v, ok := <-ch` | 接收一个值，`ok=false` 表示 channel 已关闭且缓冲区已空 | 需要区分"零值"与"关闭" |
| `for v := range ch` | 反复接收，直到 channel 被关闭且缓冲区读空 | 消费整个流（最常用） |
| `for range ch` | 同上，但不使用收到的值 | 只关心"流结束"这个事件 |

`for v := range ch` 是最常用的消费形式，但它有一个**必须记住的前提**：**只有 `close` 才能让它退出**。如果发送方不关闭 channel，`range` 就会永久阻塞在最后一次接收上，形成 goroutine 泄漏。

```go
package main

import "fmt"

func main() {
	nums := []int{1, 2, 3, 4, 5}
	ch := make(chan int)

	// 生产者：所有数据发完后关闭 channel
	go func() {
		defer close(ch)
		for _, n := range nums {
			ch <- n * n
		}
	}()

	sum := 0
	for v := range ch { // 上游 close 后循环自然结束
		sum += v
	}
	fmt.Println("平方和 =", sum)

	// for range ch：只等结束，不关心值
	done := make(chan int, 3)
	go func() {
		defer close(done)
		for i := 0; i < 3; i++ {
			done <- i
		}
	}()
	for range done {
		// 空循环体：把 channel 排干
	}
	fmt.Println("done 已排干，len =", len(done))
}
```

**解读**：生产者用 `defer close(ch)`，无论正常结束还是中途 `return`（甚至 panic 传播前），channel 都会被关闭，消费方不会卡死。这是"生产者-消费者"的骨架，10.7 会在此基础上构建 Worker Pool。

### 10.3.7 单向 channel：让编译器帮你检查方向

channel 类型可以是单向的，并且**双向 channel 可以隐式转换为单向 channel，反过来不行**：

```go
// ⚠️ 片段：仅示意函数签名（只有声明、没有函数体，声明合法但不能调用）
func produce(out chan<- int)  // 只能发
func consume(in <-chan int)   // 只能收
```

它的价值不在运行时，而在**接口设计**：把函数签名写成单向，编译器就会替你挡住"消费者误关 channel""生产者误读结果"这类错误。

```go
package main

import "fmt"

// send 只能发送，因此它无法误读或关闭这个 channel（close 需要双向 chan）
func send(out chan<- int, n int) {
	for i := 1; i <= n; i++ {
		out <- i
	}
	close(out) // ✅ close 允许作用于发送方向
}

// recv 只能接收，因此它无法误发或关闭
func recv(in <-chan int) (sum int) {
	for v := range in {
		sum += v
	}
	return sum
}

func main() {
	ch := make(chan int, 4) // 双向 channel
	go send(ch, 4)          // 隐式转成 chan<- int
	fmt.Println("总和 =", recv(ch))
}
```

**解读**：`close` 在单向 channel 上是允许的，但只能在**发送方向**上（`chan<- T`）；在 `<-chan T` 上调 `close` 是编译错误 `invalid operation: close(ch) (cannot close receive-only channel)`。这正是我们想要的编译期保护。

💡 **提示**：单向 channel 的常见误用是"声明一个单向 channel 变量"：

```go
// ⚠️ 片段：错误示范（会永久阻塞，不要运行）
var ch chan<- int
ch <- 1 // 永久阻塞（nil channel）
```

单向 channel 几乎**只应该出现在函数参数与返回值位置**，创建时必须用双向的 `make(chan T)`，再靠隐式转换传进去。

### 10.3.8 channel 的底层结构：hchan

理解 `hchan` 是理解"为什么 channel 是引用类型""为什么无缓冲和有缓冲行为不同"的关键。运行时中的结构可以简化成这样（`runtime/chan.go`）：

```text
type hchan struct {
    qcount   uint           // 缓冲区里现有的元素个数（len(ch) 读它）
    dataqsiz uint           // 缓冲区容量（cap(ch) 读它）
    buf      unsafe.Pointer // 指向环形缓冲区，dataqsiz 个元素
    elemsize uint16         // 元素大小
    closed   uint32         // 是否已关闭
    elemtype *_type         // 元素类型（用于反射、GC）
    sendx    uint           // 下一次发送写入的下标
    recvx    uint           // 下一次接收读取的下标
    recvq    waitq          // 阻塞的接收者队列（sudog 链表）
    sendq    waitq          // 阻塞的发送者队列（sudog 链表）
    lock     mutex          // 保护以上所有字段
}
```

（这是**简化版**：Go 1.26 的 `runtime/chan.go` 里 `hchan` 还多了 `timer *timer`（配合 `time.Timer` 的 channel 用法）与 `bubble *synctestBubble`（配合 `testing/synctest`）两个字段，它们不影响下面这些结论。字段顺序也与实际源码略有出入，这里按"便于理解"重排过。）

由此可以解释一系列行为：

- **为什么 channel 是引用类型。** `make(chan T)` 返回的是一个指向 `hchan` 的指针。函数间传 `chan T` 传的是这个指针，所以各方操作的是同一个 `hchan`——传值也能共享。
- **为什么是 FIFO。** 环形缓冲区 `buf` 配合 `sendx`/`recvx` 两个游标，天然是先入先出。当缓冲区满时，发送者会被打包成 `sudog` 挂到 `sendq`；有接收者取走一个元素后，运行时会从 `sendq` 里取一个发送者，**直接把它的数据写进缓冲区空出的位置**（而不是先入队再出队），既保证 FIFO 又省一次拷贝。
- **为什么无缓冲 channel 是同步的。** `dataqsiz == 0`，`buf` 为空。发送操作发现 `recvq` 里有等待者时，直接把数据**从发送者的栈拷贝到接收者的栈**（这一步在运行时里叫 `send`/`recv` 的快速路径）；如果没有等待者，就把自己挂到 `sendq` 并让出 CPU。
- **为什么 `len(ch)` 在并发下不可靠。** 它读的是 `qcount`，是一个瞬时值；读完的下一纳秒就可能被别的 goroutine 改变。
- **为什么 channel 里有一把锁。** 每次发送/接收都要短暂获得 `hchan.lock`（在快速路径上是一次轻量的自旋/原子操作），所以 channel **本身就是用锁实现的**——"用 channel 就不用锁"只是把锁藏进了运行时。

### 10.3.9 channel 的性能量级

下面的数字都是**数量级**，用于建立直觉，不要当成精确基准（不同 CPU、不同负载差异很大）：

| 操作 | 典型耗时量级 | 说明 |
| --- | --- | --- |
| 原子操作（atomic） | 数纳秒 | 单条 CPU 指令，无调度 |
| `Mutex` 无竞争 Lock/Unlock | 十几到几十纳秒 | 一次 CAS |
| 无竞争的有缓冲 channel 收发 | 几十纳秒 | 一次 CAS + 一次加锁 |
| 无缓冲 channel 收发（需配对唤醒） | 百纳秒级 | 可能涉及 `gopark`/`goready` |
| 有竞争的 channel / mutex | 微秒级甚至更高 | 涉及 goroutine 挂起与唤醒 |

结论有三条：

1. **channel 比 mutex 慢，但语义更强。** 一次可用的 goroutine 唤醒/挂起（约百纳秒到微秒）和一次 `Mutex` 的 CAS（约十几纳秒）不是同一个量级。
2. **先选语义正确的工具，再谈优化。** 用 mutex 保护一个计数器，比开一个专门的 goroutine 通过 channel 串行化所有自增要快得多，也简单得多。
3. **热路径上的高频小操作别用 channel。** 比如"每处理一个元素就通过 channel 发一个统计计数"，在百万 QPS 下会成为瓶颈；此时用 `atomic.Int64` 或"本地累加 + 定期 flush"更合适。

## 10.4 Select 多路复用

`select` 是 Go 并发编排的"总调度台"：它让一个 goroutine 同时等待多个 channel 操作，谁先就绪就执行谁。

### 10.4.1 语法与随机选择机制

```text
select {
case v := <-ch1:   // 接收
case ch2 <- v:     // 发送
case <-ch3:        // 接收并忽略值
default:           // 可选：所有分支都不就绪时执行
}
```

`select` 的执行规则：

1. 先对所有 case 中的 channel 表达式**求值一次**（注意：这里也包括 `default` 之外的"被选择的 channel"，但发送/接收操作本身要等到选中才执行）。这就是 10.4.4 中"把 channel 置 nil 来禁用分支"能生效的原因。
2. 如果有多个 case 就绪，运行时会**随机选一个**执行。
3. 如果没有 case 就绪：有 `default` 就执行 `default`；没有 `default` 就阻塞，直到至少一个 case 就绪。

**为什么是随机的？** 因为"随机"是**公平性**的实现手段。试想 `case <-ch` 与 `case <-ctx.Done()` 同时就绪：如果 `select` 总是选第一个，那么取消信号可能被饿死，程序无法及时退出。更常见的场景是两个负载 channel：永远按固定顺序选择会让一个 channel 的数据被饿死或让缓冲区爆掉。运行时通过打乱轮询顺序（`pollorder`，用 `cheaprand` 生成随机排列）来实现近似均匀的公平选择。注意：**"随机"不是"统计上严格均匀"的承诺**，它只保证不会出现系统性的固定偏好。

```go
package main

import "fmt"

func main() {
	a := make(chan int, 1)
	b := make(chan int, 1)
	a <- 1
	b <- 2

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		select { // 两个 case 都始终就绪 → 选择是随机的
		case <-a:
			counts["a"]++
			a <- 1 // 立刻把刚取空的那个补满，保证下一轮两个 case 依然都就绪
		case <-b:
			counts["b"]++
			b <- 2
		}
	}
	fmt.Printf("a=%d b=%d（两者都接近 500，说明没有固定偏向）\n", counts["a"], counts["b"])
}
```

**解读**：每次循环先选一个（随机），再把两个 channel 都补满，保证下一轮两个 case 都就绪。所以 1000 次之后两个计数都在 500 上下——**依赖 `select` 的顺序来解决优先级问题一定会失败**，这正是 10.4.5 要讲的内容。

### 10.4.2 `default` 实现非阻塞操作

`default` 分支的存在与否，决定了 `select` 是"非阻塞尝试"还是"阻塞等待"：

- **有 `default`**：所有 case 都不就绪时立刻执行 `default` → 非阻塞。
- **无 `default`**：阻塞直到某个 case 就绪。

最经典的应用是**非阻塞接收**（正确的"先检查再读"姿势，代替不可靠的 `len(ch) > 0`）：

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 2)
	ch <- 10
	ch <- 20

	// 用 select+default 排干 channel，不阻塞
	for {
		select {
		case v := <-ch:
			fmt.Println("取到", v)
		default:
			fmt.Println("channel 已空，退出")
			return
		}
	}
}
```

另一个高频用法是**非阻塞发送**：进程内统计、日志、埋点这类"丢了也不影响正确性"的数据，用 `default` 直接丢弃，绝不阻塞业务逻辑：

```go
// 片段：省略 package main 与 import，仅示意关键代码
select {
case metrics <- sample:
	// 成功投递
default:
	// 队列已满，丢弃本次采样（这是有意的降级策略）
}
```

**❌ 错误用法：用 `default` 实现"忙等"。**

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄
for {
	select {
	case v := <-ch:
		handle(v)
	default:
		// ❌ 什么都不做 → 死循环，CPU 100%
	}
}
```

**现象**：即使 channel 一直没数据，这个循环也会疯狂占用一个 P，`top` 里能看到 100% 的 CPU 占用，同时什么都没干。**✅ 正确用法**：只在"确实要立刻做别的事"时使用 `default`；纯粹的等待应该去掉 `default` 让 goroutine 挂起（挂起是不消耗 CPU 的），或者加一个 `time.After` 做节流。

### 10.4.3 `time.After` 实现超时

超时是并发编程中"必须有的兜底"。`time.After(d)` 返回一个 `<-chan time.Time`，在 d 之后收到一个时间值，因此可以直接用在 `select` 里：

```go
package main

import (
	"fmt"
	"time"
)

// fetch 模拟一次可能很慢的请求
func fetch(d time.Duration) string {
	time.Sleep(d)
	return "数据"
}

func main() {
	done := make(chan string, 1)

	go func() { done <- fetch(500 * time.Millisecond) }()

	select {
	case v := <-done:
		fmt.Println("成功拿到：", v)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("超时：100ms 内没有结果，放弃等待")
	}
}
```

**解读**：三点要注意。

1. **`done` 必须是有缓冲的（容量 1），或至少在超时后仍有人能接收。** 如果是无缓冲 channel，超时后主 goroutine 走了，那个发送方 goroutine 会永久阻塞在 `done <- ...` 上——一次典型的 goroutine 泄漏。给结果 channel 加一个容量就是最省事的解法（这叫"结果槽"模式）。
2. **超时只代表"放弃等待"，不代表"工作已经停止"。** 上面那个 goroutine 还会继续跑完 `fetch`。要真正取消工作，需要把 `context` 传进去（第 11 章），让被调用的函数自己检查取消。
3. **热循环里别用 `time.After`。** Go 1.23 起未引用的定时器可以被 GC 回收，问题比过去小得多，但在每秒数十万次的循环里，每次创建一个 `Timer` 仍然是可观的开销。替代方案是 `time.NewTimer` + `Reset`，或直接给整个循环用一个 `context.WithTimeout`。

**一个完整的"超时 + 取消"骨架**，可以直接套用到实际项目里：

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// doWork 是真正干活的函数：它必须监听 ctx，否则取消只是"表面取消"
func doWork(ctx context.Context) (int, error) {
	select {
	case <-time.After(300 * time.Millisecond): // 模拟耗时工作
		return 42, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel() // 及时释放 context 内部的定时器资源

	res, err := doWork(ctx)
	if err != nil {
		fmt.Println("任务未完成：", err) // context deadline exceeded
		return
	}
	fmt.Println("结果：", res)
}
```

### 10.4.4 空 `select{}`、`for` + `select` 与禁用分支

**空 `select{}` 永久阻塞。** `select {}` 没有任何 case，也没有 `default`，因此它永远阻塞，且运行时会**不会**把它当作死锁报错（因为它可能被其他 goroutine 唤醒——实际上不会，但运行时的判定只看"是否所有 goroutine 都睡着"，而空 select 会让 main goroutine 睡着，如果其他 goroutine 还活着，程序就继续跑）。典型用途是"让 main 挂起，等后台 goroutine 干完活"：

```go
// ⚠️ 片段：故意永久阻塞——补上 package main 就是一个永不退出的程序，
// 只用于生产服务的 main 结尾，学习与测试中不要这样写
func main() {
	go func() {
		// 长期运行的服务（HTTP server 等）
	}()
	select {} // 挂起 main，永不返回
}
```

⚠️ **注意**：这种写法在**生产代码**里可以用（很多服务的 `main` 结尾确实这么写），但在**学习示例和测试里是禁忌**——它会让程序无法正常退出，`go run` 只能靠 Ctrl+C 中断。本书所有示例都保证能自行结束。

**`for` + `select` 是 Go 的"事件循环"标准骨架**，每一个长期运行的 goroutine 都应该长这样：

```go
// 片段：省略 package main 与 import，仅示意关键代码
for {
	select {
	case v, ok := <-in:
		if !ok {
			return // 上游关闭，正常退出
		}
		handle(v)
	case <-ticker.C:
		flush()
	case <-ctx.Done():
		return // 取消，退出
	}
}
```

三个要点：**必须有退出路径**（`ok == false` 或 `ctx.Done()`）；**不要在里面做长时间阻塞的操作**（否则会漏掉取消信号）；**如果内部有耗时任务，把它放到新的 goroutine 或用 `select` 保护**。

**禁用分支：把 channel 变量置 nil。** 如 10.3.5 所述，`select` 中操作数为 nil channel 的分支永远不就绪，等价于"这段 case 被注释掉了"。这个技巧在"必须处理完全部数据源才能退出"的循环里非常关键：

```go
// 片段：省略 package main 与 import，仅示意关键代码
for len(pending) > 0 {
	select {
	case v := <-a:
		// 处理 a，若 a 已关闭则置 a = nil
	case v := <-b:
		// 处理 b，若 b 已关闭则置 b = nil
	}
}
```

如果不用置 nil 的技巧，每次 `case <-a` 被选中时读到的都是"已关闭 channel"的零值，循环会变成 CPU 空转——这就是 10.9 陷阱表里"关闭后仍被 select 反复选中"的成因。

### 10.4.5 select 与优先级

如 10.4.1 所验证的，`select` 会随机选择就绪的分支，**它本身不提供优先级**。要实现"优先处理高优先级 channel，只有在高优先级没数据时才处理低优先级"，必须**显式嵌套两轮 `select`**：

```go
package main

import "fmt"

// recvPriority 优先消费 high；只有 high 空时才碰 low。
// 返回 (值, 来源)，两个 channel 都空时返回 ok=false。
func recvPriority(high, low <-chan string) (string, string, bool) {
	// 第一轮：只尝试 high（非阻塞）
	select {
	case v := <-high:
		return v, "high", true
	default:
	}
	// 第二轮：high 没数据，再同时等 high 和 low
	select {
	case v := <-high:
		return v, "high", true
	case v := <-low:
		return v, "low", true
	default:
		return "", "", false
	}
}

func main() {
	high := make(chan string, 1)
	low := make(chan string, 3)
	high <- "高优先级任务"
	low <- "低优先级 1"
	low <- "低优先级 2"

	for {
		v, src, ok := recvPriority(high, low)
		if !ok {
			break
		}
		fmt.Printf("[%s] %s\n", src, v)
	}
	fmt.Println("队列已空，退出")
}
```

**解读**：第一轮用 `default` 做"只试 high"的探测；只有在 high 确实空的情况下才进入第二轮。这样只要有高优先级数据，就一定先被处理。

**注意语义边界**：这里实现的是"**在每个调度点优先**"，不是"**全局严格优先**"。如果在两次调用之间 high 来了数据，已经进入第二轮的 `select` 仍可能选中 low——这是不可避免的，除非你愿意用更复杂的"先排干 high"的策略。工程上常见的折中方案是"**批量排空高优先级通道**"（用一个内层 `for` 把 high 读干），再处理 low。

### 10.4.6 用 select 实现超时、取消、心跳与限流

`select` 是四个高频并发需求的基础设施，这里给出可直接使用的形态。

**心跳（Heartbeat）**：定期向外部证明"我还活着"。

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	beat := make(chan time.Time, 1)

	// 生产者：每 30ms 发一次心跳
	go func() {
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop() // 必须 Stop，否则定时器持续占用资源
		for t := range ticker.C {
			select {
			case beat <- t:
			default: // 没人来得及消费就跳过这次心跳，不阻塞心跳循环
			}
		}
	}()

	deadline := time.After(100 * time.Millisecond)
	beats := 0
	for {
		select {
		case <-beat:
			beats++
		case <-deadline:
			fmt.Println("收到心跳次数：", beats)
			return
		}
	}
}
```

**限流（信号量式的并发上限）**：用有缓冲 channel 当"令牌桶"，容量就是并发上限。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	const maxConcurrent = 2
	sem := make(chan struct{}, maxConcurrent) // 信号量：容量 = 并发上限

	var wg sync.WaitGroup
	for i := 1; i <= 6; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sem <- struct{}{}        // 获取令牌，满了就在这里排队（天然限流）
			defer func() { <-sem }() // 释放令牌

			fmt.Printf("任务 %d 开始（当前并发 ≤ %d）\n", id, maxConcurrent)
			time.Sleep(20 * time.Millisecond)
		}(i)
	}
	wg.Wait()
	fmt.Println("全部任务完成")
}
```

**解读**：`sem <- struct{}{}` 与 `<-sem` 必须成对，用 `defer` 释放是唯一安全的写法（否则 panic 会永久吃掉一个令牌）。这一模式在 10.7.4 会有更细致的讨论。

**取消**：所有可能长时间阻塞的地方都加上 `case <-ctx.Done()`，就是"可取消"的全部秘密。

## 10.5 同步原语（sync 包）

channel 适合"流动的数据"，`sync` 包适合"共享的状态"。本节按"错误用法 → 现象 → 正确用法"的顺序过一遍最常用的原语。在此之前先记住一条贯穿全节的规则：

> **`sync` 包中的所有类型都含有 `noCopy` 标记，绝不能被拷贝。** 拷贝检测交给 `go vet`（`copylocks` 检查），它是这类 bug 的唯一有效防线。

### 10.5.1 `sync.Mutex`

`Mutex`（mutual exclusion，互斥锁）保证同一时刻只有一个 goroutine 进入临界区。

```go
// 片段：省略 package main 与 import，仅示意关键代码
var mu sync.Mutex
mu.Lock()
// 临界区：这里对共享变量的访问是安全的
mu.Unlock()
```

**零值可用。** `var mu sync.Mutex` 就是一个可用的锁，不需要初始化。这也是"把 Mutex 作为结构体字段"而不是"指向 Mutex 的指针"成为主流写法的原因。

**`TryLock`（Go 1.18+）。** 它在锁空闲时获取锁并返回 `true`，否则**立即返回 `false`**，不阻塞。它的语义非常明确："**我只在能立刻拿到锁的时候才做，否则就不做**"。它绝不应该被用来实现"带超时的锁"——标准的做法是用 channel 或 `context`。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// counter 是一个并发安全的计数器，演示 Mutex 的三种正确用法
type counter struct {
	mu sync.Mutex // 零值即可用；不要写成指针，也不需要初始化
	n  int
}

// Inc 是常规用法：加锁 → 临界区 → 解锁
func (c *counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock() // defer 保证 panic 也能解锁
	c.n++
}

// IncIfIdle 演示 TryLock：拿不到锁就直接放弃，绝不阻塞
func (c *counter) IncIfIdle() bool {
	if !c.mu.TryLock() {
		return false
	}
	defer c.mu.Unlock()
	c.n++
	return true
}

// Get 演示 RWMutex 缺席时的读：普通 Mutex 的读也要加锁
func (c *counter) Get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func main() {
	c := &counter{}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Go(func() { c.Inc() })
	}
	wg.Wait()
	fmt.Println("100 次并发自增后：", c.Get())

	// TryLock：故意制造竞争，观察"能拿到就做，拿不到就跳过"
	hold := make(chan struct{})
	var mu sync.Mutex
	mu.Lock()
	go func() {
		<-hold
		mu.Unlock()
	}()
	fmt.Println("锁被占用时 TryLock 返回：", mu.TryLock()) // false，且不会卡住
	close(hold)
	time.Sleep(10 * time.Millisecond)             // 等上面那个 goroutine 解锁
	fmt.Println("锁空闲后 TryLock 返回：", mu.TryLock()) // true
	mu.Unlock()
}
```

**解读**：`TryLock` 的返回值是"是否成功"，不是"锁是否可用"，注意别写反。

**❌ 错误用法一：值接收者导致锁被拷贝。**

```go
package main

import (
	"fmt"
	"sync"
)

type badCounter struct {
	mu sync.Mutex
	n  int
}

// Inc 用的是值接收者 → c 是结构体的副本，锁和 n 都是副本
func (c badCounter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++ // 改的是副本，调用方一无所知
}

func main() {
	c := badCounter{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() { c.Inc() })
	}
	wg.Wait()
	fmt.Println("结果：", c.n) // 结果：0
}
```

**现象**：无论并发多少次，`c.n` 永远是 0——"加了锁却什么都没发生"。更可怕的是这个程序**看起来完全正确**：有 `Lock`、有 `defer Unlock`、有 `WaitGroup`。`go vet` 会给出决定性提示：`Inc passes lock by value: main.badCounter contains sync.Mutex`。**✅ 正确用法**：所有涉及锁的方法都用**指针接收者**（`func (c *counter) Inc()`），并在结构体层面就确定"这个类型只能通过指针传递"。

**❌ 错误用法二：不可重入导致的自我死锁。** Go 的 `Mutex` **不是可重入锁**（non-reentrant）。同一个 goroutine 已经持有锁时再次 `Lock`，会永久阻塞：

```text
❌ 会死锁的写法（不要运行）
func (c *counter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.n++
    c.log()        // 内部又调用了 c.mu.Lock() → 同一个 goroutine 卡死自己
}
```

**现象**：程序卡住，`SIGQUIT` 后能看到 `semacquire` 的栈——同一个 goroutine 既在等锁又是锁的持有者。对比 Java 的 `ReentrantLock`/`synchronized`，Go 明确选择了"不提供可重入"，理由是**可重入会掩盖锁边界不清的设计问题**。**✅ 正确用法**：把加锁的粒度收窄到最小方法，或者拆出一个**假定已持锁**的内部方法（命名如 `incLocked`，注释说明调用者必须持锁）：

```go
// 片段：省略 package main 与 import，仅示意关键代码
func (c *counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.incLocked() // 内部方法不重复加锁
}

func (c *counter) incLocked() { c.n++ } // 调用者必须已持有 c.mu
```

**❌ 错误用法三：重复 `Unlock`。** `Unlock` 一个未加锁的 `Mutex` 会触发 **fatal error**：

```text
fatal error: sync: unlock of unlocked mutex
```

**现象**：注意它是 `fatal error` 而不是 `panic`——**`recover` 无法捕获它**，程序直接崩溃。它由运行时用 `throw` 抛出，因为"解锁未加的锁"意味着锁的使用逻辑已经彻底错乱，继续运行毫无意义。同理，"拷贝一个已经加锁的 Mutex"在 `-race` 下会被检测出来。**✅ 正确用法**：`Lock` 与 `defer Unlock` 成对出现，中间不要有跨函数的复杂跳转；`defer` 是防止"某条分支忘记解锁"的最可靠手段。

**`defer Unlock` 的性能取舍。** `defer` 本身在内联优化后开销极小（Go 1.14 起开放编码的 defer，纳秒级），**在绝大多数代码里应当无条件使用 `defer mu.Unlock()`**。只有在极端热点的、已确认瓶颈的短临界区里，才考虑手动 `Unlock`：

```go
// 片段：省略 package main 与 import，仅示意关键代码
// 极热点：手动解锁，省掉 defer 的少量开销，但必须保证所有路径都解锁
func (c *counter) FastInc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}
```

**判断标准**：先写 `defer`，用 pprof 确认锁的开销占比后再改。为了省几纳秒而引入"某条 return 忘记解锁导致整个服务卡死"的风险，是本末倒置。

⚠️ **注意**：不要嵌入 `sync.Mutex` 到公开结构体（`type Cache struct { sync.Mutex; ... }`），那会把 `Lock`/`Unlock` 提升为公开 API，让调用者有能力绕过你的封装。用命名字段 `mu sync.Mutex`。

### 10.5.2 `sync.RWMutex`

`RWMutex` 提供"多读单写"语义：

| 方法 | 语义 |
| --- | --- |
| `RLock()` / `RUnlock()` | 共享的读锁，多个 goroutine 可同时持有 |
| `Lock()` / `Unlock()` | 排他的写锁，与其他读锁、写锁互斥 |

适用条件是**读远多于写**（经验阈值是读写比 10:1 以上），且**读操作本身不能太短**——如果临界区只有一条指令，`RWMutex` 内部更复杂的记账（原子计数、信号量）反而比 `Mutex` 慢，因为：`Mutex.Lock` 一次 CAS 就完事，而 `RLock` 要维护读者计数和写者等待标志。

**写饥饿与 Go 1.9 的改进。** 早期实现中，源源不断的读者可能让写者永远拿不到锁（写者饥饿）。Go 1.9 之后改为**写者优先**：一旦有写者在等待，新的 `RLock` 会被阻塞，排到写者后面。这消除了写饥饿，但引入一个必须知道的副作用——**递归读锁的死锁风险**：

```text
❌ 会死锁的写法（不要运行）
func (c *Config) Outer() string {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.Inner()      // Inner 里又要 RLock
}

func (c *Config) Inner() string {
    c.mu.RLock()          // 若此刻已有写者在等待，这次 RLock 会阻塞
    defer c.mu.RUnlock()  // → 而 Outer 持有的读锁又不释放 → 死锁
    return c.data
}
```

**现象**：单测里几乎永远跑不出来，只在"恰好有写等待 + 恰好发生嵌套读"时死锁——**典型的线上偶发事故**。**✅ 正确用法**：把"假设已持读锁"的内部方法拆出来（`getLocked`），绝不在持读锁时再去 `RLock`；用 `-race` 也检测不到这类问题（它不是数据竞争），只能靠 `SIGQUIT` 栈或死锁检测工具定位。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// configCenter 是读多写少的典型场景：读非常频繁，写很罕见
type configCenter struct {
	mu   sync.RWMutex
	data map[string]string
}

func newConfigCenter() *configCenter {
	return &configCenter{data: make(map[string]string)}
}

// Get 走读锁：多个 goroutine 可以同时读
func (c *configCenter) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[key]
	return v, ok
}

// Set 走写锁：排他修改
func (c *configCenter) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

func main() {
	c := newConfigCenter()
	c.Set("mode", "prod")

	var wg sync.WaitGroup
	// 100 个并发读者
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			if v, ok := c.Get("mode"); ok {
				_ = v // 真实业务里会用到；这里只用来说明读路径
			}
		})
	}
	// 少量写者
	wg.Go(func() { c.Set("mode", "dev") })
	wg.Go(func() { c.Set("version", "1.26") })
	wg.Wait()

	mode, _ := c.Get("mode")
	version, _ := c.Get("version")
	fmt.Printf("最终配置：mode=%s version=%s\n", mode, version)

	// 读多写少的量级感受：顺序执行 20 万次读，读锁的开销远低于写锁
	start := time.Now()
	for i := 0; i < 200000; i++ {
		c.Get("mode")
	}
	fmt.Println("20 万次读锁耗时量级：", time.Since(start).Round(time.Millisecond))
}
```

**解读**：`RWMutex` 的另一个常见误用是"**用读锁保护写操作**"——比如在 `RLock` 里修改 map，多个读者同时写同一个 map 会直接触发运行时检测：`fatal error: concurrent map writes`（这是运行时主动检测，与数据竞争检测无关）。记住：**只要临界区内有写，就必须用写锁**，与"业务上大多数时候是读"无关。

### 10.5.3 `sync.WaitGroup`（补充：复用与竞态）

`Add`/`Done`/`Wait` 的基本用法见 10.2.4，这里补充两条与同步语义直接相关的注意点：

- **计数器归零之前不能再次 `Add`**（指与上一轮 `Wait` 并发的情形）。如果一定要循环复用，模式是"每轮任务全部 `Done` → `Wait` 返回 → 才启动下一轮"。
- **`Add` 的负值必须谨慎**。`Add(-1)` 合法（等价 `Done`），但如果计数器已经是 0 再 `Add(-1)`，会 panic：`sync: negative WaitGroup counter`。这是"多调了一次 `Done`"的典型症状。

### 10.5.4 `sync.Once`

`Once` 保证某个函数在多 goroutine 环境下**只执行一次**，是单例初始化、全局资源懒加载的标准工具。

```go
package main

import (
	"fmt"
	"sync"
)

var (
	once     sync.Once
	instance *dbConn
)

type dbConn struct{ dsn string }

// GetDB 是并发安全的懒加载单例
func GetDB() *dbConn {
	once.Do(func() {
		fmt.Println("只执行一次：建立数据库连接")
		instance = &dbConn{dsn: "postgres://localhost/app"}
	})
	return instance
}

func main() {
	var wg sync.WaitGroup
	conns := make([]*dbConn, 50)
	for i := range conns {
		wg.Go(func() { conns[i] = GetDB() })
	}
	wg.Wait()

	allSame := true
	for _, c := range conns {
		if c != conns[0] {
			allSame = false
		}
	}
	fmt.Println("50 个 goroutine 拿到同一个实例：", allSame)
	fmt.Println("dsn：", GetDB().dsn)
}
```

**解读**：`once.Do(f)` 的并发保证很强：**所有调用者都会阻塞到 f 执行完毕**，因此 `Do` 返回之后，`instance` 对所有人都是可见的（这是 Go 内存模型提供的 happens-before 保证）。这比"双检锁 + 原子标记"更简洁，也更不容易写错。

🔥 **重点陷阱：`Do` 中的 panic 会让后续调用"静默跳过"。**

> 官方文档明确说明：**如果 `f` 发生 panic，`Do` 会认为它已经返回；后续对 `Do` 的调用会直接返回，而不会再调用 `f`。**

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var once sync.Once

	f := func() {
		fmt.Println("初始化开始……")
		panic("初始化失败：配置读不到")
	}

	try := func(round int) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("第 %d 次调用捕获到：%v\n", round, r)
			}
		}()
		once.Do(f)
		fmt.Printf("第 %d 次调用：Do 正常返回（注意 f 并没有再执行）\n", round)
	}

	try(1) // panic 被捕获，但 Once 已经"认为完成了"
	try(2) // f 不会被再次调用，Do 直接返回
	try(3)
}
```

**输出**：

```text
初始化开始……
第 1 次调用捕获到：初始化失败：配置读不到
第 2 次调用：Do 正常返回（注意 f 并没有再执行）
第 3 次调用：Do 正常返回（注意 f 并没有再执行）
```

**现象**：第 2、3 次调用**既不报错也不重试**，直接拿到一个**未初始化完成**的状态（在上面的单例例子里就是 `instance == nil`），后续使用会以"空指针 panic"或"数据错误"的形式在离现场很远的地方爆发。**✅ 正确用法**：`Do` 里的函数**不要 panic**。把错误通过包级变量记录下来，让调用方检查：

```go
// 片段：省略 package main 与 import，仅示意关键代码
var (
	once    sync.Once
	initErr error
	cfg     *Config
)

func Load() (*Config, error) {
	once.Do(func() {
		cfg, initErr = loadConfig() // 把错误"存起来"，而不是 panic 出去
	})
	return cfg, initErr
}
```

💡 **提示**：Go 1.21 起标准库提供了三个更简洁的包装：`sync.OnceFunc(f)` 返回一个"只执行一次"的 `func()`；`sync.OnceValue(f)` 返回一个"只求值一次并缓存结果"的 `func() T`；`sync.OnceValues(f)` 对应两个返回值的情形。它们的 panic 语义与 `Once.Do` 相同，但省掉了手写包装，值得优先使用。

### 10.5.5 `sync.Map`

`sync.Map` 是一个**为特定场景优化的并发 map**，它的 API 围绕"键值对操作"设计：`Store`、`Load`、`LoadOrStore`、`LoadAndDelete`、`Delete`、`Range`、`Swap`、`CompareAndSwap`、`CompareAndDelete`，以及 **Go 1.23 新增的 `Clear()`**（清空全部条目；网上不少资料把它误记为 1.20，实际是 1.23 引入的）。

**它的内部实现经历过一次重写，这一点直接决定了它的适用场景。** 早期的 `sync.Map` 采用"**只读快照（read map）+ 可写脏 map（dirty map）**"的双结构：读操作优先走只读快照，用原子方式完成，完全不加锁；未命中或需要写入时才升级到加锁路径，并可能把脏 map 提升为新的只读快照。**从 Go 1.24 起，`sync.Map` 改为对 `internal/sync.HashTrieMap`（并发哈希前缀树，hash-trie）的封装**：内部用不可变节点 + 原子指针替换来实现"读不加全局锁"，负载因子与扩容策略也更接近普通 map。这次重写的主要收益是**在较大的 map 上做写与删除也不再明显退化**，因此官方文档中那条"key 集合必须稳定"的老建议已经不像过去那么严格。

即便如此，选择逻辑没有变，它**默认仍然不是普通 map 的替代品**：

| 适用 | 不适用 |
| --- | --- |
| 读远多于写（典型的"热读缓存"） | 写远多于读 |
| 多个 goroutine 读写**不相交**的 key 集合 | 需要"读-改-写"原子复合操作（应用 `LoadOrStore`/`CompareAndSwap`，或换回 Mutex） |
| 想省掉一把全局锁、又不想自己做分片（sharding） | 需要 `len()`（`sync.Map` **没有** `Len` 方法，只能 `Range` 数） |
| 想做"每连接/每租户一小份状态"的缓存 | 需要类型安全（key/value 都是 `any`，每次都要断言） |

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var m sync.Map
	var wg sync.WaitGroup

	// 并发写入不相交的 key
	for i := 0; i < 100; i++ {
		wg.Go(func() { m.Store(i, i*i) })
	}
	wg.Wait()

	if v, ok := m.Load(9); ok {
		fmt.Println("Load(9) =", v)
	}

	// LoadOrStore：不存在才写入，返回实际生效的值与"是否已存在"
	actual, loaded := m.LoadOrStore(9, 999)
	fmt.Printf("LoadOrStore(9, 999) → actual=%v loaded=%v（已存在，不加锁改写）\n", actual, loaded)

	// LoadAndDelete：读取并删除
	if v, ok := m.LoadAndDelete(9); ok {
		fmt.Println("LoadAndDelete(9) =", v)
	}
	_, ok := m.Load(9)
	fmt.Println("删除后 Load(9) 是否命中：", ok)

	// Range：遍历期间没有一致性快照保证，不要依赖"遍历时的值一定最新"
	count := 0
	m.Range(func(k, v any) bool {
		count++
		return true // 返回 false 会提前终止遍历
	})
	fmt.Println("剩余条目数：", count)

	m.Clear() // Go 1.23+
	after := 0
	m.Range(func(k, v any) bool { after++; return true })
	fmt.Println("Clear 之后条目数：", after)
}
```

**解读**：三个关键认知。

1. **`sync.Map` 不是 `map` 的替代品。** 它是"针对特定访问模式的优化结构"，普通场景下的第一选择仍然是 `Mutex`（或 `RWMutex`）+ 内置 `map`；Go 官方文档的表述是"大多数代码应该使用普通的 map 配合锁或 goroutine 隔离"，因为那样类型安全、复合操作简单、行为可预测。
2. **`Range` 不提供一致性快照。** 遍历期间可能有并发的增删，`Range` 不保证看到所有 key，也不保证每个 key 只被看到一次。需要一致性快照就自己加锁 + 复制。
3. **`sync.Map` 的值是 `any`，付出的是类型断言与逃逸开销。** 如果你的 key 和 value 类型固定且要压榨性能，泛型封装（`sync.Map` 外面包一层类型安全的 wrapper）或自己写分片锁（sharded mutex map）往往更好。

**`Mutex` + `map` vs `sync.Map` 的取舍**：

| 维度 | `Mutex` + `map` | `sync.Map` |
| --- | --- | --- |
| 类型安全 | 强（`map[string]*Conn`） | 弱（`any`，需断言） |
| 读多写少 | 尚可（读也要加锁，读越频繁竞争越明显） | **最优（读路径几乎无锁）** |
| 写多、key 频繁新增 | **更优**（单锁下写法直白） | 已明显改善（1.24 起为 hash-trie），但复合写入往往要写 CAS 循环 |
| 复合操作（判断+修改） | 加锁下原子完成，简单 | 需要 `LoadOrStore`/`CompareAndSwap` 组合，较绕 |
| 代码可读性 | 高 | 中（API 面较大） |

### 10.5.6 `sync.Pool`

`sync.Pool` 是**临时对象缓存**：把用完的对象放回去，下次直接取出来复用，从而减少分配次数与 GC 压力。它的核心特征是——**不保证任何对象会被保留**。

- `Get()`：从池中取一个对象；池为空时调用 `New`（若设置了 `New`），否则返回 `nil`。
- `Put(x)`：把对象放回池中。
- `New func() any`：可选字段，池为空时的生产函数。

**生命周期由 GC 决定。** 每次 GC 时池中的对象可能被"顺走"（Go 1.13 起引入了 victim cache，池中对象通常能挺过一次 GC，但第二次基本就没了）。所以：

> **`sync.Pool` 里的对象不能携带必须保留的状态；它只是一个"用得上就省一次分配，用不上也无所谓"的优化。**

**适合放进池里的东西**：临时的 `bytes.Buffer`、`[]byte` 缓冲区、`encoding/json` 的编码器/解码器、需要重复分配的大型结构体切片。
**不适合的**：持有连接、持有需要 `Close()` 的资源、需要精确状态的对象（因为取出来时状态未知）、体积极小且分配成本极低的对象（`int`、小 struct，池的记账开销反而更大）。

```go
package main

import (
	"bytes"
	"fmt"
	"sync"
)

// bufPool 复用 bytes.Buffer，避免每次格式化都分配新缓冲区
var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer) // 池为空时的兜底：分配一个新的
	},
}

// decorate 用池中的缓冲区拼装一行输出
func decorate(name string) string {
	buf := bufPool.Get().(*bytes.Buffer) // 取出后必须断言回具体类型
	defer func() {
		buf.Reset()      // ✅ 归还前清理状态：不复位等于把脏数据传给下一个使用者
		bufPool.Put(buf) // 放回池中
	}()

	buf.WriteString("<")
	buf.WriteString(name)
	buf.WriteString(">")
	return buf.String() // 注意：Bytes() 会返回别名切片，Reset 后内容会被覆盖
}

func main() {
	var wg sync.WaitGroup
	out := make([]string, 100)
	for i := range out {
		wg.Go(func() { out[i] = decorate(fmt.Sprintf("item-%d", i)) })
	}
	wg.Wait()

	fmt.Println(out[0], out[49], out[99])
	fmt.Println("复用成功：没有出现串数据说明 Reset 生效")
}
```

**解读**：这里有两个经典陷阱。第一，**必须 `Reset()` 再 `Put`**，否则下一个使用者会读到上一个使用者残留的数据（数据串号，且在安全场景下可能是信息泄漏）。第二，**不要把 `buf.Bytes()` 的结果带出去**——它返回的是指向缓冲区内部的切片，对象被复用后这块内存会被改写。要带出数据就用 `buf.String()`（`[]byte → string` 会复制）或者显式 `append([]byte(nil), buf.Bytes()...)`。

### 10.5.7 `atomic` 包

原子操作是不可分割的 CPU 指令（如 x86 的 `LOCK CMPXCHG`），它是**最轻量的同步手段**：无锁、无 goroutine 挂起、纳秒量级。

**传统函数式 API**（`sync/atomic`，Go 1.19 之前的主流写法）：`AddInt64`、`LoadInt64`、`StoreInt64`、`SwapInt64`、`CompareAndSwapInt64`，以及对应的 `Int32`、`Uint32`、`Uint64`、`Pointer` 版本。

**类型化 API（Go 1.19+，推荐用法）**：`atomic.Int64`、`atomic.Int32`、`atomic.Uint64`、`atomic.Bool`、`atomic.Pointer[T]` 等。它们把"被原子操作的值"封装成类型，方法名更短，并且**内部对齐由类型保证**，从而彻底消除了 32 位平台上"64 位变量必须 64 位对齐"这个古老的坑（在 32 位平台上，一个未对齐的 64 位原子操作会直接 panic）。

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// server 展示三种类型化原子操作的推荐写法
type server struct {
	// 计数器：高频自增，用 atomic.Int64 而不是 Mutex
	requests atomic.Int64
	// 开关：一次写入、多次读取
	healthy atomic.Bool
	// 快照指针：读写分离的配置热更新
	config atomic.Pointer[config]
}

type config struct {
	name string
	qps  int
}

func main() {
	s := &server{}
	s.healthy.Store(true)
	s.config.Store(&config{name: "v1", qps: 100})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			s.requests.Add(1) // 等价于 old := requests; requests = old+1，但是原子的
		})
	}
	wg.Wait()
	fmt.Println("总请求数：", s.requests.Load())
	fmt.Println("健康状态：", s.healthy.Load())

	// CompareAndSwap：只在当前值等于期望值时才替换（自旋/乐观并发的核心指令）
	swapped := s.healthy.CompareAndSwap(true, false)
	fmt.Printf("CompareAndSwap(true, false) → %v，当前值 %v\n", swapped, s.healthy.Load())
	swapped = s.healthy.CompareAndSwap(true, false) // 当前已是 false，交换失败
	fmt.Printf("再次 CompareAndSwap(true, false) → %v，当前值 %v\n", swapped, s.healthy.Load())

	// Swap：无条件替换并返回旧值
	old := s.healthy.Swap(true)
	fmt.Printf("Swap(true) 返回旧值 %v，当前值 %v\n", old, s.healthy.Load())

	// atomic.Pointer：整体替换配置快照，读路径完全无锁
	oldCfg := s.config.Swap(&config{name: "v2", qps: 500})
	cur := s.config.Load()
	fmt.Printf("配置从 %s 热更新到 %s（qps %d → %d）\n", oldCfg.name, cur.name, oldCfg.qps, cur.qps)
}
```

**解读**：`atomic.Pointer[T]` 是"不可变快照 + 原子替换"模式的核心，非常适合配置热更新：读路径只需一次原子 Load 加解引用，**零锁开销**；写路径构造一个全新的配置对象再整体 Swap，绝不会让读者看到"改了一半"的状态。这个模式在 Kubernetes、etcd 等项目中大量出现。

🔥 **重点：原子操作的内存序语义。** Go 的 `sync/atomic` 提供的是**顺序一致性（Sequentially Consistent）**语义：所有原子操作看起来按照某个全局全序执行，并且这个顺序与每个 goroutine 的程序顺序一致。这意味着 Go 的原子操作**不需要**像 C++ 那样手动选择 `memory_order_acquire`/`release`——这是 Go 为了易用性做的取舍，代价是某些平台上的潜在性能损失。

但必须清楚它的**边界**：

> **原子操作只保护"通过原子操作访问的那一个变量"。** 它不提供临界区，也不保护其他内存。

```text
❌ 典型误解
var data atomic.Pointer[record]   // 用原子指针保护 *record
p := data.Load()                  // 原子读指针 ✓
p.field++                         // ❌ 对 p 指向的结构体字段的写，不是原子操作！
```

如果多个 goroutine 都执行 `p.field++`，那依然是数据竞争。**✅ 正确姿势**：要么让 `record` 变成**不可变对象**（只读，改的时候整个替换指针），要么对字段的访问另外用锁或原子操作保护。这就是"不可变快照"模式流行的原因。

⚠️ **注意**：`atomic.Int64` 等类型同样**不能被拷贝**（内部有 `noCopy`），并且不要把它们作为值在函数间传递——所有方法都是指针接收者。

### 10.5.8 `sync.Cond`

`sync.Cond`（条件变量）用于"等待某个条件成立"。它是本章这些同步原语里最难用、也最容易用错的一个。

```go
// 片段：省略 package main 与 import，仅示意关键代码
c := sync.NewCond(&mu)  // 必须绑定一个 Locker（通常就是保护条件的那把锁）
c.L.Lock()
for !condition() {      // 必须用 for，不能用 if
	c.Wait()            // 内部：解锁 → 挂起 → 被唤醒后重新加锁
}
// 此处条件成立，且已重新持有锁
c.L.Unlock()

c.Signal()     // 唤醒一个等待者
c.Broadcast()  // 唤醒所有等待者
```

**四个必须掌握的点**：

1. **必须配合锁使用。** `Cond` 不自己管理锁，它需要一个 `Locker`（`sync.Locker` 接口，即 `Lock`/`Unlock`）。检查条件与 `Wait` 之间必须连续持有锁，否则会丢失唤醒（lost wakeup）。
2. **`Wait` 内部会解锁再重新加锁。** 调用 `Wait` 前必须已持有 `c.L`；`Wait` 会释放它让别的 goroutine 能改条件，被唤醒后再重新获取。这解释了为什么 `Wait` 返回后你能安全地读条件变量，也解释了为什么不能在未加锁时调用 `Wait`——`Cond.Wait` 的第一步就是 `c.L.Unlock()`，对一把没上锁的 `Mutex` 执行 `Unlock` 会触发 **`fatal error: sync: unlock of unlocked mutex`**（注意是 fatal error，`recover` 无法捕获）。
2. **必须用 `for` 循环重新检查条件**。注意：Go 的 `Cond` **没有** C/Java 意义上的虚假唤醒（spurious wakeup）——官方文档明确写着"`Wait` 只有在被 `Broadcast` 或 `Signal` 唤醒时才会返回"。但循环仍然是必须的，原因有两条：一是 `Signal` 唤醒的是**任意一个**等待者，被唤醒的那个未必是条件已经成立的那个；二是从"发出 `Signal`"到"`Wait` 真正重新拿到锁"之间存在窗口期，条件可能又被别的 goroutine 改了回去。官方文档的原文就是"caller typically cannot assume that the condition is true when Wait returns"。
4. **优先用 channel。** Go 官方文档明确建议：**"在绝大多数情况下，用 channel 或 `sync` 包的其他类型比 `Cond` 更好。"** 只有在"一个条件被大量 goroutine 共享、且不想为每个等待者都建一个 channel"的场景下，`Cond` 才显得顺手（例如实现一个阻塞队列的内部通知）。

```go
package main

import (
	"fmt"
	"sync"
)

// queue 用 sync.Cond 实现一个有界阻塞队列，展示 Cond 的标准用法
type queue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond // 条件：缓冲区不空
	notFull  *sync.Cond // 条件：缓冲区不满
	items    []int
	cap      int
	closed   bool
}

func newQueue(capacity int) *queue {
	q := &queue{cap: capacity}
	q.notEmpty = sync.NewCond(&q.mu) // 两个条件变量共用同一把锁
	q.notFull = sync.NewCond(&q.mu)
	return q
}

func (q *queue) Push(v int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 用 for 等待"不满"：循环检测条件，抵御虚假唤醒与"被抢跑"
	for len(q.items) >= q.cap && !q.closed {
		q.notFull.Wait()
	}
	if q.closed {
		return false
	}
	q.items = append(q.items, v)
	q.notEmpty.Signal() // 有新元素了，唤醒一个消费者
	return true
}

func (q *queue) Pop() (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 {
		if q.closed {
			return 0, false
		}
		q.notEmpty.Wait() // 等待"不空"
	}
	v := q.items[0]
	q.items = q.items[1:] // 注意：这里只移动切片头，底层数组会被复用
	q.notFull.Signal()    // 腾出位置了，唤醒一个生产者
	return v, true
}

func (q *queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.notEmpty.Broadcast() // 必须广播：所有等待者都要醒来检查 closed
	q.notFull.Broadcast()
}

func main() {
	q := newQueue(2)

	var wg sync.WaitGroup
	// 生产者：写入 0..9 后关闭队列
	wg.Go(func() {
		defer q.Close()
		for i := 0; i < 10; i++ {
			if !q.Push(i) {
				return
			}
		}
	})

	// 两个消费者：各自把队列取空
	sum := make([]int, 2)
	for c := 0; c < 2; c++ {
		wg.Go(func() {
			for {
				v, ok := q.Pop()
				if !ok {
					return // 队列已关闭且取空
				}
				sum[c] += v // 每个消费者只写自己的下标，无竞态
			}
		})
	}
	wg.Wait()
	fmt.Println("两个消费者各自求和（互不干扰）：", sum, "总计", sum[0]+sum[1])
}
```

**解读**：`Signal` 与 `Broadcast` 的选择有一条简单规则——**"唤醒一个就够"用 `Signal`，"状态变化让所有等待者都可能满足条件"用 `Broadcast`**。上面的 `Close` 必须用 `Broadcast`，因为它改变了"所有等待者都关心的全局状态"。另外注意 `q.items = q.items[1:]` 这种"假出队"会让底层数组持续被复用（切片头后移，容量不变），在长时间的队列里会导致内存无法释放——生产代码里应改成环形缓冲区或定期重整。

⚠️ **注意**：`Cond` 没有 `TryWait`，也没有超时版本。需要超时就只能用 channel + `select`。这也是"优先用 channel"这条建议最有力的理由。

## 10.6 数据竞争（Data Race）

### 10.6.1 定义：三个条件同时成立

**数据竞争**的严格定义是：**两个或更多的 goroutine 并发访问同一个内存位置，其中至少有一次是写操作，并且这些访问之间没有任何同步关系。** 三个条件缺一不可：

1. **同一内存位置**：同一个变量、同一个结构体字段、同一个切片元素（注意：`s[i]` 与 `s[j]` 且 `i != j` 时不是同一位置，所以"每个 goroutine 只写自己的下标"是安全的）。
2. **至少一次是写**：两个 goroutine 都只读同一个变量，永远不是竞态（这也是 `RWMutex` 读锁有意义的前提）。
3. **没有同步关系**：既没有锁、也没有 channel、也没有原子操作、也没有 `WaitGroup` 等建立 happens-before。

容易被忽略的是第 3 条的"**happens-before**"性质：光有"时间上先后"不算同步。下面这段代码看起来"先写完再启动 goroutine，肯定没问题"，实际上 race detector 会报错——因为启动 goroutine 这个动作本身确实建立了 happens-before（`go` 语句之前的写在子 goroutine 中可见），但如果写发生在 `go` 之后，就完全没有保证：

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄
data := 0
go func() { fmt.Println(data) }() // ✅ go 之前的 data=0 是可见的
data = 1                          // ❌ 这次写与上面的读是竞态
```

```go
package main

import (
	"fmt"
	"sync"
)

// 这段程序存在数据竞争：1000 个 goroutine 同时执行 counter++
func main() {
	counter := 0 // 普通变量，没有任何保护

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Go(func() {
			counter++ // = 读 → 加 1 → 写，三步之间可能被别的 goroutine 插入
		})
	}
	wg.Wait()

	fmt.Println("期望 1000，实际可能是多少？：", counter)
}
```

**现象**：`counter` 的值**几乎不可能等于 1000**，典型结果是 96x~99x 之间的某个数，且每次运行都不同。原因是 `counter++` 在机器层面是"读入寄存器 → 加 1 → 写回内存"三条指令，两个 goroutine 可能都读到 100，各自加 1 后都写回 101——**一次自增被吃掉了**。跑 `go run -race main.go` 会立刻得到一份数据竞争报告。

### 10.6.2 用 `-race` 检测：完整输出解读

race detector（数据竞争检测器）基于 Google 的 ThreadSanitizer，使用方式：

```bash
go run -race main.go          # 运行单个程序
go test -race ./...           # 测试（最重要：让测试覆盖并发路径）
go build -race -o app.exe .   # 构建带检测的二进制
go vet ./...                  # 静态检查（copylocks、loopclosure 等）
```

⚠️ **注意**：race detector **需要启用 cgo 并有可用的 C 编译器**。Linux/macOS 上通常开箱可用；Windows 上需要安装 MinGW-w64 之类的 GCC 工具链并把 `CGO_ENABLED=1` 打开。如果环境不具备，`go run -race` 会直接报错——这是环境问题，不是代码问题。

在上面的计数器程序上运行 `go run -race main.go`，会得到形如下面的输出（**这是 race detector 的标准输出格式**）：

> ⚠️ **关于下面的输出**：它是 race detector 的**格式示意**，不是在本机实测得到的——race detector 需要 cgo 与 C 编译器（Windows 上通常是 MinGW-w64 的 gcc），本书的验证环境里没有装 gcc，因此无法实际运行 `-race`。**请读者在自己的机器上按上面的命令跑一遍并对照**；地址、goroutine 编号、`+0x...` 偏移每次运行都不同，行号则取决于你在 `counter++` 前后加了多少注释。

```text
==================
WARNING: DATA RACE
Read at 0x00c00001a0b8 by goroutine 8:
  main.main.func1()
      D:/demo/main.go:15 +0x2c

Previous write at 0x00c00001a0b8 by goroutine 7:
  main.main.func1()
      D:/demo/main.go:15 +0x44

Goroutine 8 (running) created at:
  main.main()
      D:/demo/main.go:14 +0x90

Goroutine 7 (finished) created at:
  main.main()
      D:/demo/main.go:14 +0x90
==================
Found 1 data race(s)
exit status 66
```

**逐行解读**：

| 输出行 | 含义 |
| --- | --- |
| `WARNING: DATA RACE` | 检测到一次数据竞争，一次报告只描述**一对**冲突访问 |
| `Read at 0x00c00001a0b8 by goroutine 8:` | **冲突的一方**：goroutine 8 在地址 `0x00c00001a0b8` 执行了读（`0x...` 是运行时地址，每次运行都不同） |
| `main.main.func1()` + 文件行号 | 冲突发生的调用栈：匿名函数 `main.main.func1` 在 `main.go` 第 15 行，也就是 `counter++` 那一行 |
| `+0x2c` | 该位置在函数内的指令偏移，用于区分同一行的不同指令 |
| `Previous write at ... by goroutine 7:` | **冲突的另一方**：goroutine 7 对**同一地址**执行了写 |
| `Goroutine 8 (running) created at:` | goroutine 8 的**创建点**（`main.main` 第 14 行的 `wg.Go(...)`）。括号里的状态（`running` / `finished`）是报告时刻该 goroutine 的状态 |
| `Found 1 data race(s)` | 本次运行共发现几处竞争（可能一次运行发现多处） |
| `exit status 66` | race detector 默认在发现竞争时让程序以退出码 **66** 结束（可用 `GORACE=exitcode=...` 调整），这让你可以在 CI 里用"退出码非 0"直接卡住流水线 |

**读报告的正确顺序**：先看两处栈的**文件:行号**（这是你的代码，比运行时地址有用得多），再看 `created at` 推断是谁启动的这个 goroutine，最后看这一对访问是"读-写"还是"写-写"。上例中两方都在第 15 行 `counter++`，说明是典型的"读改写"竞争。

**CI 集成**：把 `go test -race ./...` 作为必需步骤，并设置 `GORACE="halt_on_error=1"` 让程序在第一次竞争时立即终止（默认是继续执行并汇总报告）。同时注意 `-race` 会让内存占用显著上升，`GORACE="history_size=7"` 可以调大历史缓冲区以提高发现率（代价是更多内存）。

### 10.6.3 竞态修复的四种写法对比

同一个"并发计数器"需求，有四种正确或错误的实现。下面这个程序把它们放在一起，用同一份负载验证结果：

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	goroutines = 1000
	perG       = 1000
)

// ① 无保护：错误示范
func plainCounter() int {
	n := 0
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Go(func() {
			for j := 0; j < perG; j++ {
				n++ // ❌ 数据竞争
			}
		})
	}
	wg.Wait()
	return n
}

// ② atomic：最轻量
func atomicCounter() int64 {
	var n atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Go(func() {
			for j := 0; j < perG; j++ {
				n.Add(1) // ✅ 单条原子指令，无锁
			}
		})
	}
	wg.Wait()
	return n.Load()
}

// ③ Mutex：把整个批次的循环放进临界区
func mutexCounter() int {
	var (
		mu sync.Mutex
		n  int
	)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Go(func() {
			mu.Lock()
			for j := 0; j < perG; j++ {
				n++ // ✅ 临界区内，安全
			}
			mu.Unlock() // 锁粒度大：整个 1000 次自增占住锁
		})
	}
	wg.Wait()
	return n
}

// ④ channel：把"所有权"交给一个专职 goroutine
func channelCounter() int {
	ch := make(chan int)
	done := make(chan int)
	go func() {
		n := 0
		for range ch { // 只有这个 goroutine 碰 n，天然无竞态
			n++
		}
		done <- n
	}()

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Go(func() {
			for j := 0; j < perG; j++ {
				ch <- 1
			}
		})
	}
	wg.Wait()
	close(ch) // 关闭后上面的 range 结束，再回传结果
	return <-done
}

func main() {
	expect := goroutines * perG
	fmt.Printf("期望值：%d\n", expect)
	fmt.Printf("① 无保护（可能有误）：%d\n", plainCounter())
	fmt.Printf("② atomic：          %d\n", atomicCounter())
	fmt.Printf("③ Mutex：           %d\n", mutexCounter())
	fmt.Printf("④ channel：         %d\n", channelCounter())
}
```

**四种方案的性质对比**：

| 方案 | 正确性 | 性能量级 | 代码清晰度 | 适用场景 |
| --- | --- | --- | --- | --- |
| 普通变量 | ❌ 数据竞争 | —— | 看起来最简单 | 绝不使用 |
| `atomic.Int64` | ✅ | 最快（纳秒级） | 高 | 单一数值的增减、标志位、指针替换 |
| `Mutex` | ✅ | 中等（取决于临界区） | 高 | 一次要保护**多个**相关字段的复合修改 |
| `channel` | ✅ | 最慢（含调度） | 中（要设计收尾） | 数据需要"交由某一方独占"、需要背压或编排 |

三条实践结论：

1. **能原子就原子，需要复合就加锁，需要所有权转移才用 channel。** 上例中 channel 版本比 atomic 慢一个数量级以上，而它做的事和 atomic 完全一样——**为同一个计数器用 channel 是典型的过度设计**。
2. **锁粒度决定 Mutex 的性能。** 上面 `mutexCounter` 把 1000 次自增都放在一次加锁内（粗粒度），实际反而比"每次自增加一次锁"快得多，因为锁竞争次数少了 1000 倍。**临界区应该"尽量短"指的是不包含无关的耗时操作（I/O、Sleep），而不是把批处理拆成逐条加锁。**
3. **`-race` 是验证正确性的唯一自动化手段。** 上面第 ① 种写法在不开 `-race` 时只是"数字不对"，在开 `-race` 时会明确报出竞争位置。

### 10.6.4 "未定义行为"到底意味着什么

很多资料把数据竞争轻描淡写成"结果可能是错的"，这远远不够。准确的说法是：**存在数据竞争的程序，其行为在 Go 内存模型下是未定义的（undefined behavior）**。具体含义：

- **不只是"某次自增丢失"。** 编译器可以认为"没有任何其他 goroutine 会修改这个变量"，从而把循环里的 `n++` 提升成一个寄存器自增、最后只写一次内存；也可以把一个读操作提前到循环外。于是"竞态"可能表现为**结果错得离谱**（比如 1000 个 goroutine 各加 1000 次，结果却是 1000），而不是"少加了几次"。
- **可能"看起来正常"。** 在单核、`GOMAXPROCS=1`、或者 CPU 缓存恰好一致的条件下，竞态程序可能长期跑出正确结果。这就是为什么"我测了一百遍都没问题"不能作为证据——它只是还没触发。
- **可能崩溃，而且崩溃位置与 bug 无关。** 竞态可能破坏 map 的内部状态（触发 `fatal error: concurrent map writes` 或 `concurrent map read and map write`）、破坏切片的 `len`/`cap`、让指针指向已释放对象。此时 panic 的位置和真正的 bug 相隔十万八千里，排查成本极高。
- **不是"偶尔出错"，而是"完全不可预测"。** 一次"偶尔出错"至少说明概率可估计、可以复现；未定义行为意味着在任何时刻、任何平台、任何编译器版本下都可能出现**任何**行为——包括**今天正确、明天换了 CPU 就崩溃**。

🔥 **重点**：因此正确的态度是——数据竞争是**必须消除的 bug 类别**，而不是"概率问题"。这条规则没有例外：**任何一条存在数据竞争的程序，都不能算"能工作"，只能算"还没爆"。**

### 10.6.5 race detector 的局限

race detector 是必需品，但不是万能药：

| 局限 | 说明 | 应对 |
| --- | --- | --- |
| **只能发现实际执行到的竞争** | 它是动态检测器，必须运行到那两处访问才会报告。单元测试没覆盖的分支、只在特定并发时序下出现的交错，它看不到 | 提高并发测试覆盖率；在压测/集成环境也开 `-race`；用 `-race` + `-count` 多跑几轮 |
| **有显著性能与内存开销** | 典型是 CPU 慢数倍、内存多几倍，因此**不建议在生产开启** | 只在测试与预发环境开启 |
| **不检测死锁、活锁、goroutine 泄漏** | 这些不是内存竞争，需要别的工具（`pprof`、超时、`SIGQUIT` 栈） | 结合 goroutine profile 与超时机制 |
| **不检测"逻辑竞态"** | 数据无竞争但业务语义错误（例如"先查余额再扣款"两步之间没有锁，虽然每次访问都加锁了，整体仍不是原子的）依然存在 | 设计时明确复合操作的原子边界，必要时把"检查+修改"放进同一个临界区 |
| **不能替代设计** | 它不会告诉你"这个并发结构本身有泄漏风险" | 遵循本章的所有权与生命周期原则 |

## 10.7 并发模式入门

本章只给"基础版"，更复杂的编排（流水线、`context` 树、errgroup、单飞、限速器、并发安全的缓存体系）留到第 11 章。

### 10.7.1 生产者-消费者

这是所有并发结构的基本单元。骨架只有四条规则：

1. 生产者负责**发送**，发完负责 **`close`**（唯一的关闭者）。
2. 消费者用 **`for range`** 消费，靠 `close` 自然退出。
3. 需要取消时，双方都监听同一个 `ctx.Done()`。
4. 需要"等全部结束"时，用 `WaitGroup` 等所有生产者/消费者。

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// produce 每隔一段时间产生一个数，直到达到 n 或收到取消信号
func produce(ctx context.Context, n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out) // 唯一的发送方 → 由它关闭
		for i := 1; i <= n; i++ {
			select {
			case out <- i:
			case <-ctx.Done(): // 消费者提前退出时，生产者不会卡在发送上
				return
			}
		}
	}()
	return out
}

// consume 消费直到上游关闭；只想要前 limit 个
func consume(ctx context.Context, in <-chan int, limit int) []int {
	got := make([]int, 0, limit)
	for v := range in {
		got = append(got, v)
		if len(got) == limit {
			return got // 主动提前退出，靠 ctx 通知上游
		}
	}
	return got
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	var got []int
	wg.Go(func() {
		got = consume(ctx, produce(ctx, 100), 3) // 只取 3 个，剩下的由 cancel 收尾
	})
	wg.Wait()
	cancel() // 通知生产者停下

	time.Sleep(20 * time.Millisecond) // 仅为演示：让生产者的退出路径跑完
	fmt.Println("消费者拿到：", got)
}
```

**解读**：这个例子的重点在于**提前退出的正确收尾**。消费者只要 3 个就返回了，如果生产者还在往无人接收的 channel 里发送，它就会永久阻塞。`select` 里的 `case <-ctx.Done()` 是唯一的解药——**任何可能无人接收的发送，都必须可被取消**。

### 10.7.2 Worker Pool（固定 worker + 任务 channel + 结果 channel）

Worker Pool 是服务端最常用的并发模式：**把"无限并发"改成"受控并发"**。它解决三个问题：限制资源占用、复用 worker（避免为每个任务创建 goroutine）、给上游提供背压。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

type job struct {
	id  int
	val int
}

type result struct {
	jobID  int
	out    int
	worker int // 由谁完成，仅用于观察负载分布
}

// worker 是标准的 worker 循环：
//  1. for range jobs 消费任务，jobs 被 close 后自然退出；
//  2. 每完成一个任务就把结果写进 results；
//  3. 退出前 Done()，让收尾 goroutine 知道可以关闭 results 了。
func worker(id int, jobs <-chan job, results chan<- result, wg *sync.WaitGroup) {
	defer wg.Done()
	for j := range jobs {
		time.Sleep(5 * time.Millisecond) // 模拟耗时计算
		results <- result{jobID: j.id, out: j.val * j.val, worker: id}
	}
}

func main() {
	const (
		numJobs    = 24
		numWorkers = 4
	)

	jobs := make(chan job, numWorkers)    // 任务队列：缓冲一小段即可，天然背压
	results := make(chan result, numJobs) // 结果队列：容量等于任务数，避免消费者慢导致 worker 卡住

	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(w, jobs, results, &wg)
	}

	// 投递任务
	for i := 1; i <= numJobs; i++ {
		jobs <- job{id: i, val: i}
	}
	close(jobs) // 关键一步：关闭后所有 worker 的 range 都会结束

	// 收尾 goroutine：等所有 worker 退出后再关闭 results
	// 不能由 main 直接 close(results)，因为此时可能还有 worker 在发送 → panic
	go func() {
		wg.Wait()
		close(results)
	}()

	sum, count := 0, 0
	byWorker := map[int]int{}
	for r := range results { // results 被关闭后循环结束
		sum += r.out
		count++
		byWorker[r.worker]++
	}

	fmt.Printf("完成 %d 个任务，平方和 = %d\n", count, sum)
	fmt.Printf("负载分布（共 %d 个 worker）：%v\n", numWorkers, byWorker)
}
```

**解读**：这段代码有五个必须理解的细节。

1. **`close(jobs)` 是 worker 的退出信号**。少了它，`for range jobs` 永远不结束，`wg.Wait()` 永远不返回，整个程序死锁。
2. **`close(results)` 必须由"等所有 worker 都退出"的那个 goroutine 来做**。因为 `results` 有多个发送方（每个 worker 都是），而"只能由发送方关闭"且"只能关一次"。如果 `main` 直接 `close(results)`，还在运行的 worker 一发送就会 `panic: send on closed channel`。
3. **`results` 的容量取 `numJobs`**，让 worker 不必等消费者。实际项目中如果结果很大，应该改成有界缓冲 + 消费者足够快，或者让消费者数量与 worker 数量匹配。
4. **worker 数量不是越多越好**。经验值是 CPU 密集任务取 `runtime.NumCPU()`，I/O 密集任务取"下游能承受的并发数"（例如数据库连接池大小）。**worker 数超过下游容量只会把压力转移到下游，而不是提高吞吐。**
5. **`byWorker` 的分布取决于任务耗时是否均匀**。示例里每个任务都固定睡 5 毫秒、任务数又刚好能被 worker 数整除，所以实测输出是很整齐的 `map[1:6 2:6 3:6 4:6]`；一旦任务耗时差异很大（有的瞬间完成、有的要几百毫秒），先启动的 worker 就会抢到更多任务，分布会明显倾斜。这不代表调度有问题——**work stealing 发生在"任务被领取"层面，而不是"任务被均分"层面**。

### 10.7.3 Fan-out / Fan-in

- **Fan-out（扇出）**：多个 goroutine 从**同一个** channel 读，把工作并行化。
- **Fan-in（扇入）**：把**多个** channel 合并成**一个** channel，把结果汇聚起来。

上面 Worker Pool 里"4 个 worker 读同一个 `jobs`"就是 Fan-out。Fan-in 的通用工具函数长这样：

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// fanIn 把多个 channel 合并成一个：每个源一个转发 goroutine，全部结束后关闭输出
func fanIn(sources ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup

	for _, src := range sources {
		wg.Add(1)
		go func(s <-chan int) {
			defer wg.Done()
			for v := range s { // 源关闭后本转发 goroutine 结束
				out <- v
			}
		}(src)
	}

	// 收尾：所有源都转发完毕后才关闭 out
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// gen 生成一串数
func gen(name string, n int) <-chan int {
	ch := make(chan int)
	go func() {
		defer close(ch)
		for i := 1; i <= n; i++ {
			ch <- i
			time.Sleep(time.Millisecond) // 模拟生产耗时
		}
	}()
	return ch
}

func main() {
	merged := fanIn(gen("A", 3), gen("B", 3), gen("C", 3))

	sum, count := 0, 0
	for v := range merged { // 只有 fanIn 关闭 out 后，这里才会结束
		sum += v
		count++
	}
	fmt.Printf("扇入收到 %d 个数，总和 = %d\n", count, sum)
}
```

**解读**：Fan-in 的正确性完全依赖那个"收尾 goroutine"——它把 `WaitGroup` 与 `close` 串起来，保证 `out` 只在**所有**源都转发完后才关闭。少了这个收尾，`for range merged` 会永久阻塞；把它写错（比如某个源自己的 goroutine 里 `close(out)`），就会出现 `send on closed channel`。

⚠️ **注意**：Fan-in 会**打乱顺序**。上面这三个源的数据是交错到达的，最终输出顺序不可预测。需要保序的设计必须额外工作（给每条数据打序号 + 缓冲重排，或者干脆串行化）。

### 10.7.4 用 channel 实现信号量与互斥锁

**信号量（Semaphore）：容量为 N 的有缓冲 channel。** 缓冲区里的每个元素代表一个"可用名额"：

```go
// 片段：省略 package main 与 import，仅示意关键代码（含 ... 占位，不能直接编译）
sem := make(chan struct{}, 3) // 最多 3 个并发

sem <- struct{}{}             // P：获取名额（满了就阻塞 → 这就是限流）
// ... 做需要限流的工作 ...
<-sem                         // V：释放名额
```

它比 `sync.Mutex` 强的地方在于"**N > 1 的并发上限**"，这是 mutex 做不到的。下面是一个"限制对下游服务的并发请求数"的完整实现：

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// limiter 用有缓冲 channel 实现并发上限，并记录历史最大并发数用于验证
type limiter struct {
	sem  chan struct{}
	cur  atomic.Int64
	peak atomic.Int64
}

func newLimiter(n int) *limiter { return &limiter{sem: make(chan struct{}, n)} }

// Acquire 获取一个名额；Release 释放。
// 一定要用 defer Release()，否则 panic 会永久泄漏一个名额。
func (l *limiter) Acquire() {
	l.sem <- struct{}{}
	now := l.cur.Add(1)
	for { // 用 CAS 更新峰值，避免额外加锁
		old := l.peak.Load()
		if now <= old || l.peak.CompareAndSwap(old, now) {
			break
		}
	}
}

func (l *limiter) Release() {
	l.cur.Add(-1)
	<-l.sem
}

func main() {
	const maxConcurrent = 3
	l := newLimiter(maxConcurrent)

	var wg sync.WaitGroup
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			l.Acquire()
			defer l.Release() // 用 defer，任何路径都能归还名额

			time.Sleep(10 * time.Millisecond) // 模拟下游调用
		}(i)
	}
	wg.Wait()

	fmt.Printf("限制上限 = %d，实际观测到的最大并发 = %d（不应超过上限）\n",
		maxConcurrent, l.peak.Load())
}
```

**用 channel 实现互斥锁：容量为 1 的 channel。** 信号量取 N=1 就是互斥锁：

```go
// 片段：省略 package main 与 import，仅示意关键代码
var mu = make(chan struct{}, 1) // 容量 1 的 channel = 互斥锁

mu <- struct{}{}                // Lock
// --- 临界区 ---
<-mu                            // Unlock
```

**它与 `sync.Mutex` 的区别**：

| 维度 | 容量 1 的 channel | `sync.Mutex` |
| --- | --- | --- |
| 语义 | 获取不到就阻塞（与 mutex 相同） | 获取不到就阻塞 |
| 能放进 `select` | ✅ 可以，能搭配超时、取消、`default` | ❌ 不能 |
| 性能 | 略慢（多一层 `hchan` 的逻辑） | 更快 |
| 能否"试锁" | `select` + `default` 即试锁 | `TryLock`（Go 1.18+） |
| 零值可用 | ❌ 必须 `make` | ✅ 零值即可用 |

**唯一值得用 channel 当锁的理由是"需要和 `select` 组合"**，例如"最多等 100ms 拿不到锁就放弃"：

```go
// 片段：省略 package main 与 import，仅示意关键代码
select {
case mu <- struct{}{}:
	defer func() { <-mu }()
	// 拿到锁
case <-time.After(100 * time.Millisecond):
	return fmt.Errorf("获取锁超时")
}
```

这是 `sync.Mutex` 做不到的（`TryLock` 只能立刻返回，不能等待指定时长）。

## 10.8 并发安全的数据结构实战

本节把前面的原语组合成四个可以直接放进项目的组件。每个组件都给出了"为什么这么设计"的理由，而不只是能跑的代码。

### 10.8.1 带过期时间的并发安全缓存

需求：多 goroutine 读写、每个条目有存活时间（TTL）、过期条目要能被清理、能被优雅关闭（否则清理 goroutine 泄漏）。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

type entry[V any] struct {
	val      V
	expireAt time.Time
}

// Cache 是并发安全、带 TTL 的缓存。
// 用泛型是为了让调用方不必做类型断言，同时避免 sync.Map 的 any 开销。
type Cache[V any] struct {
	mu       sync.RWMutex
	items    map[string]entry[V]
	ttl      time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once // 保证 Close 可以被多次调用而不 panic
	wg       sync.WaitGroup
	hits     int64
	misses   int64
}

func NewCache[V any](ttl time.Duration) *Cache[V] {
	c := &Cache[V]{
		items:  make(map[string]entry[V]),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	c.wg.Add(1)
	go c.janitor() // 后台清理 goroutine
	return c
}

func (c *Cache[V]) Set(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry[V]{val: v, expireAt: time.Now().Add(c.ttl)}
}

// Get 命中返回 (值, true)；不存在或已过期返回 (零值, false)。
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock() // 先释放读锁：后面可能的删除要拿写锁，不能在读锁里升级

	if !ok {
		c.misses++
		var zero V
		return zero, false
	}
	if time.Now().After(e.expireAt) {
		c.mu.Lock()
		delete(c.items, key) // 惰性删除：读路径顺手清理，delete 是幂等的
		c.mu.Unlock()
		c.misses++
		var zero V
		return zero, false
	}
	c.hits++
	return e.val, true
}

func (c *Cache[V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats 返回命中与未命中次数（本示例为简化未加锁，真实代码里应放在临界区内）
func (c *Cache[V]) Stats() (hits, misses int64) { return c.hits, c.misses }

// janitor 周期性清理过期条目：解决"某些 key 写了之后再也没被读过，惰性删除永远不触发"的问题
func (c *Cache[V]) janitor() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.ttl / 2) // 清理周期取 TTL 的一半，保证过期条目最多多活半个 TTL
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sweep()
		case <-c.stopCh:
			return // 收到关闭信号，退出并 Done
		}
	}
}

func (c *Cache[V]) sweep() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.items {
		if now.After(e.expireAt) {
			delete(c.items, k) // 在 for range 中删除 key 是安全的，语义有明确定义
		}
	}
}

// Close 停止清理 goroutine 并等待它真正退出。必须调用，否则 janitor 会泄漏。
func (c *Cache[V]) Close() {
	c.stopOnce.Do(func() { close(c.stopCh) }) // 只关一次
	c.wg.Wait()                               // 等 janitor 退出
}

func main() {
	c := NewCache[string](80 * time.Millisecond)
	defer c.Close() // 保证任何路径都会关闭后台 goroutine

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() { c.Set(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)) })
	}
	wg.Wait()
	fmt.Println("写入后条目数：", c.Len())

	if v, ok := c.Get("k7"); ok {
		fmt.Println("命中 k7 =", v)
	}

	time.Sleep(120 * time.Millisecond) // 等 TTL 过期 + janitor 清理
	fmt.Println("过期并清理后条目数：", c.Len())
	c.Close()
	fmt.Println("缓存已关闭，janitor 已退出")
}
```

**解读**：这个实现里有五个值得学习的设计决策。

1. **TTL 清理用"惰性删除 + 后台扫描"双保险**。只有惰性删除，则"写完不再读"的 key 永远不释放；只有后台扫描，则读到过期值时还要等下一轮扫描。两者结合最稳。
2. **`janitor` 必须可停止**。它是最容易被忘记的泄漏源：缓存在测试里创建几百个、每个都留一个永不退出的 goroutine，`runtime.NumGoroutine()` 就会一路涨上去。
3. **`Close` 用 `sync.Once` 保护 `close(stopCh)`**。因为"关闭已关闭的 channel"会 panic，而 `Close` 常常既被 `defer` 调用、又被显式调用。
4. **`Get` 里的锁升级（读锁 → 写锁）必须先释放读锁**。`sync.RWMutex` 的读锁不能升级成写锁，直接在同一 goroutine 里再 `Lock()` 会死锁。这里选择"释放读锁后再加写锁"的写法，代价是极小的窗口内可能重复删除（`delete` 幂等，无害）。
5. **`Stats` 里的 `hits`/`misses` 未加锁是有意留的错误示范**——它正好是一个数据竞争。真实代码里应该把它们放进临界区，或改为 `atomic.Int64`。**留这个"瑕疵"是为了说明：并发安全是"每个字段都要审一遍"的全局属性，不是"加了锁就安全"。**

### 10.8.2 用 RWMutex 实现配置中心

需求：读极频繁（每个请求都要读配置）、写极罕见（运营改配置）、写完之后要通知所有订阅者。这里采用"**读锁保护数据 + 写锁下构造 + 变更后广播**"的结构：

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// Config 是不可变的配置快照：一旦发布就不允许再修改
type Config struct {
	Version int
	Timeout time.Duration
	Retries int
}

// subscriber 是一个订阅者。它的 ch 只有一个发送方——Center.Update，
// 所以 close 也必须由 Center 这一侧"代理"完成：绝不能由取消方直接 close(ch)，
// 否则并发的 Update 一旦撞上来就是 send on closed channel（见 10.3.4 的第一条规则）。
type subscriber struct {
	mu     sync.Mutex
	ch     chan *Config
	closed bool
}

// send 在锁保护下判断"是否已关闭"，保证任何时刻都不会向已关闭的 channel 发送
func (s *subscriber) send(cfg *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- cfg:
	default: // 订阅者没来得及消费就丢弃（配置是"最新值有意义"，旧值丢了无所谓）
	}
}

// stop 由取消方调用：先置标记再关闭，之后的 send 一律直接返回
func (s *subscriber) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// Center 是配置中心：读多写少，读路径只加读锁
type Center struct {
	mu   sync.RWMutex
	cur  *Config
	subs map[int]*subscriber // 订阅者：每个订阅者一个带缓冲 channel
	next int
}

func NewCenter(initial *Config) *Center {
	return &Center{cur: initial, subs: make(map[int]*subscriber)}
}

// Get 返回当前配置快照。返回指针而不是拷贝，因为 Config 是"发布后不可变"的
func (c *Center) Get() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cur
}

// Update 原子地替换整个快照，然后通知订阅者
func (c *Center) Update(timeout time.Duration, retries int) *Config {
	c.mu.Lock()
	newCfg := &Config{ // 构造一个全新的对象，不修改旧对象
		Version: c.cur.Version + 1,
		Timeout: timeout,
		Retries: retries,
	}
	c.cur = newCfg
	subs := make([]*subscriber, 0, len(c.subs))
	for _, s := range c.subs {
		subs = append(subs, s)
	}
	c.mu.Unlock() // 通知动作放在锁外，避免订阅者慢导致写路径被拖长

	for _, s := range subs {
		s.send(newCfg) // 是否已退订由 subscriber 自己把关
	}
	return newCfg
}

// Watch 注册一个订阅者，返回接收通知的 channel 与取消函数
func (c *Center) Watch() (<-chan *Config, func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.next
	c.next++
	sub := &subscriber{ch: make(chan *Config, 1)}
	c.subs[id] = sub

	cancel := func() {
		c.mu.Lock()
		delete(c.subs, id) // 先摘出注册表：后续 Update 不会再取到这个订阅者
		c.mu.Unlock()
		sub.stop() // 与 sub.send 通过 sub.mu 互斥：要么发送先做完，要么发送被跳过
	}
	return sub.ch, cancel
}

func main() {
	center := NewCenter(&Config{Version: 1, Timeout: time.Second, Retries: 3})

	// 100 个并发读者：只加读锁，彼此不互斥
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			cfg := center.Get()
			_ = cfg.Version
		})
	}
	wg.Wait()
	fmt.Println("读者看到的版本：", center.Get().Version)

	// 一个订阅者
	updates, cancelWatch := center.Watch()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for cfg := range updates {
			fmt.Printf("订阅者收到新配置：version=%d timeout=%v retries=%d\n",
				cfg.Version, cfg.Timeout, cfg.Retries)
		}
	}()

	center.Update(2*time.Second, 5)
	time.Sleep(20 * time.Millisecond) // 仅为演示：等通知送达

	cancelWatch() // 关闭订阅 channel，订阅者 goroutine 随之退出
	<-done
	fmt.Println("订阅者已退出，当前版本：", center.Get().Version)
}
```

**解读**：四个关键模式。

1. **不可变快照（immutable snapshot）**。`Update` 构造一个全新的 `*Config` 而不是修改旧对象，于是"读到旧版本"和"读到新版本"都是合法的完整状态，**不存在读到"改了一半"的中间态**。这是把 `RWMutex` 用在读路径上的前提。
2. **只在锁内改指针，不在锁内做通知**。通知涉及其他 goroutine 的调度，把它移出临界区能显著缩短写锁的持有时间。
3. **订阅者的 channel 带缓冲（容量 1）+ `select`+`default` 投递**。这保证了一个慢订阅者**永远不会阻塞配置更新**——这是一个明确的策略选择：配置是"最新值优先"的语义，积压旧值没有意义。如果语义是"每条通知都不能丢"，就必须改成阻塞投递并接受"慢订阅者拖慢写路径"的代价。
4. **关闭动作必须由"发送方"完成。** 这是本节唯一一处容易写错、而且错了就会崩的地方。天真的写法是在取消函数里 `delete(c.subs, id)` 之后直接 `close(ch)`，但 `ch` 的**发送方是 `Update`**：`Update` 把订阅者快照复制出来之后就释放了 `c.mu`，此时取消方完全可能在"快照已复制、发送尚未执行"的窗口里把 channel 关掉，于是 `Update` 一发送就 `panic: send on closed channel`。这正好踩中 10.3.4 的第一条规则——**只有发送方能关闭 channel**。

   本例的修法是给每个订阅者配一把小锁：`Update` 通过 `subscriber.send` 发送，取消方通过 `subscriber.stop` 关闭，两者在同一把 `sub.mu` 上互斥，并在 `closed` 标记上做二次确认。这样"关闭"这件事仍然是由发送方这一侧统一决策的，取消方只是发起请求。**这条经验可以推广到所有"注册-注销"式 API**：只要注销会涉及关闭 channel，就必须保证发送路径能感知到注销。

### 10.8.3 用 channel 实现无锁计数器

"无锁"在这里的准确含义是"**不使用互斥锁**"：状态的**独占所有权**被交给一个专职 goroutine，所有外部请求都通过 channel 排队进入，于是**根本不存在并发访问，也就不需要锁**。这就是 10.1.3 里"通过通信共享内存"的字面实现。

```go
package main

import (
	"fmt"
	"sync"
)

// kind 表示请求类型
type kind int

const (
	kindAdd kind = iota
	kindGet
	kindReset
)

type request struct {
	kind kind
	val  int
	done chan int // 回复通道：容量 1，避免服务方阻塞
}

// ChannelCounter 是"由单个 goroutine 独占状态"的计数器
type ChannelCounter struct {
	reqCh  chan request
	quitCh chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

func NewChannelCounter() *ChannelCounter {
	c := &ChannelCounter{
		reqCh:  make(chan request),
		quitCh: make(chan struct{}),
	}
	c.wg.Add(1)
	go c.loop()
	return c
}

// loop 是唯一接触 n 的 goroutine —— 没有共享，就没有竞态
func (c *ChannelCounter) loop() {
	defer c.wg.Done()

	n := 0
	for {
		select {
		case r := <-c.reqCh:
			switch r.kind {
			case kindAdd:
				n += r.val
				r.done <- n
			case kindGet:
				r.done <- n
			case kindReset:
				n = 0
				r.done <- 0
			}
		case <-c.quitCh:
			return // 退出，不再处理任何请求
		}
	}
}

func (c *ChannelCounter) Add(delta int) int {
	done := make(chan int, 1)
	c.reqCh <- request{kind: kindAdd, val: delta, done: done}
	return <-done
}

func (c *ChannelCounter) Value() int {
	done := make(chan int, 1)
	c.reqCh <- request{kind: kindGet, done: done}
	return <-done
}

func (c *ChannelCounter) Reset() int {
	done := make(chan int, 1)
	c.reqCh <- request{kind: kindReset, done: done}
	return <-done
}

// Close 停止后台 goroutine。可重复调用。
func (c *ChannelCounter) Close() {
	c.once.Do(func() { close(c.quitCh) })
	c.wg.Wait()
}

func main() {
	c := NewChannelCounter()
	defer c.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() { c.Add(1) })
	}
	wg.Wait()
	fmt.Println("100 次并发 Add 后：", c.Value())

	fmt.Println("再 Add(10) →", c.Add(10))
	fmt.Println("Reset →", c.Reset(), "，当前值", c.Value())
}
```

**解读**：这个模式叫"**串行化（serialization）**"或"**actor 风格**"。它的优点是**绝对不可能有数据竞争**（`n` 只有一个 goroutine 碰），缺点是**性能上限被单个 goroutine 锁死**：所有请求必须排队，一次往返包含两次 channel 操作与潜在的 goroutine 唤醒，量级远高于 `atomic.Int64` 的一次原子加。

**什么时候值得这么做**：状态转换逻辑复杂（多个字段之间有不变式）、需要集中式的顺序保证（例如"每个操作都要记日志"）、或者需要把"状态"和"协议"放在一起（比如实现一个状态机）。**什么时候不值得**：单纯的一个计数器——直接用 `atomic.Int64`。

**四种计数方案的最终对比**：

| 方案 | 性能量级 | 复合操作 | 顺序保证 | 推荐场景 |
| --- | --- | --- | --- | --- |
| `atomic.Int64` | 最快 | ❌（单变量） | 单变量原子 | 计数、标志、快照指针 |
| `sync.Mutex` | 快 | ✅ | ✅ | 多字段不变式、map 保护 |
| `sync.RWMutex` | 读快写中 | ✅ | ✅ | 读远多于写的共享状态 |
| channel 串行化 | 慢 | ✅ | ✅（全局串行） | 复杂状态机、需要集中编排 |

## 10.9 常见并发陷阱汇总

### 10.9.1 陷阱速查表

| 陷阱 | 症状 | 根因 | 正解 |
| --- | --- | --- | --- |
| **goroutine 泄漏** | `NumGoroutine` 只涨不跌，内存上涨，pprof 里同一栈堆积 | 阻塞在永不发生的条件上（发送无人接收、无退出条件的循环、等一个不关闭的 channel） | 每个 goroutine 都要有退出路径：`ctx.Done()`、`ok == false`、`close` 广播 |
| **`WaitGroup` 被拷贝** | `Wait` 永久阻塞或提前返回 | 值传递/值接收者导致计数器被复制 | 一律传 `*sync.WaitGroup`；`go vet` 会把关 |
| **`WaitGroup.Add` 位置错误** | 时好时坏，任务被"漏等" | `Add` 在 goroutine 内部，与 `Wait` 并发 | `Add` 必须在 `go` 之前；或用 `wg.Go`（Go 1.25+） |
| **循环变量捕获** | 所有 goroutine 拿到同一个值 | Go 1.22 之前循环变量是共享的 | Go 1.22+ 已修复（取决于 `go.mod` 的 `go` 指令）；显式传参最稳妥 |
| **`range` channel 永不结束** | 消费者卡在最后，goroutine 泄漏 | 发送方忘记 `close` | 唯一发送方负责 `defer close(ch)` |
| **向已关闭 channel 发送** | `panic: send on closed channel` | 多个发送方 + 某个发送方关闭了 channel | 只由"唯一的发送方"或"收尾 goroutine"关闭 |
| **重复/错误关闭** | `panic: close of closed channel` / `close of nil channel` | 多处 `close`，或 `Close()` 被调用两次 | 用 `sync.Once` 包裹 `close`；关闭前判 nil |
| **关闭后仍被 select 选中** | CPU 100%，日志刷屏零值 | 已关闭 channel 永远就绪，被反复选中 | 关闭后把该 channel 变量置为 `nil` |
| **nil channel 误用** | 永久阻塞（死锁或泄漏） | `var ch chan T` 后直接读写 | 读写的 channel 必须先 `make`；nil 只用于 `select` 禁用分支 |
| **nil map 写入** | `panic: assignment to entry in nil map` | 声明了 map 但没 `make` | `make(map[K]V)`；并发写还会触发 `fatal error: concurrent map writes` |
| **循环内 `defer Unlock`** | 锁直到函数返回才释放，并发度骤降甚至死锁 | `defer` 的执行时机是**函数**返回，不是循环体结束 | 把循环体抽成函数；或用显式 `Unlock` + 保证所有路径都解锁 |
| **锁粒度过大** | 吞吐上不去，P 利用率低 | 临界区里包含 I/O、网络调用、`Sleep` | 只把**共享状态的读写**放进临界区，把耗时操作移出去 |
| **锁粒度过小/不够** | 数据竞争、状态不一致 | "检查 + 修改"被拆成两次独立加锁 | 把复合操作整体放进同一个临界区 |
| **`defer` 在 `for` 里堆积** | 内存增长（大量未执行的 defer） | 循环里 `defer` 到函数结束才执行 | 抽函数；或改为显式调用 |
| **在持锁时调用未知函数** | 死锁 | 被调用函数内部也想拿同一把锁（可重入问题） | 明确分层：`Xxx` 加锁、`xxxLocked` 假定已持锁 |

### 10.9.2 循环变量捕获：1.22 前后的差别

这是 Go 历史上最有名的一次语义变更，必须彻底搞清。

**Go 1.22 之前**：`for i := ...` 和 `for i := range xs` 中的循环变量**整个循环只有一个**，每次迭代只是重新赋值。于是：

```text
// Go 1.21 及之前
for _, v := range []int{1, 2, 3} {
    go func() { fmt.Println(v) }()   // ❌ 三个 goroutine 大概率都打印 3
}
```

**Go 1.22 及之后**：循环变量**每次迭代都是新的**，上面的代码会打印出 `1 2 3` 的某种排列。

⚠️ **注意两个前提**：

1. **这条规则由模块的 `go` 指令决定**。`go.mod` 里写 `go 1.21` 的模块即使在 Go 1.26 的工具链下编译，仍然保持旧语义——这是 Go 团队保证向后兼容的方式。如果你的老项目行为"和网上教程不一致"，先看 `go.mod`。
2. **它只覆盖 `for` 语句声明的变量**。下面这段代码在 Go 1.26 下**仍然是错的**：

```go
// ⚠️ 片段：错误示范，省略 package main 与 import，不要照抄
v := 0
for i := 0; i < 3; i++ {
	v = i // v 是在循环外声明的，所有迭代共享同一个 v
	go func() { fmt.Println(v) }() // ❌ 依然是竞态/捕获问题
}
```

**✅ 通用正解（与语言版本无关）**：**把需要在 goroutine 里用的值通过参数传进去**，或者用局部变量显式拷贝一份：

```go
// 片段：省略 package main 与 import，仅示意关键代码
for i := 0; i < 3; i++ {
	go func(n int) { fmt.Println(n) }(i) // 传参
}

v := 0
for i := 0; i < 3; i++ {
	v = i
	n := v // 循环体内的新变量，每次迭代一份
	go func() { fmt.Println(n) }()
}
```

**检查工具**：`go vet` 的 `loopclosure` 检查能发现大部分循环变量捕获问题，配合 `go test -race` 基本可以杜绝这类 bug。

### 10.9.3 `defer` 在循环中的解锁时机

这是与锁相关的高频事故：

```text
❌ 错误写法：锁在整个循环期间都不释放
func bad(items []int) {
    mu.Lock()
    for _, it := range items {
        defer mu.Unlock()   // 真正的解锁时刻是 bad() 返回时！
        // ... 处理 it
    }
}
```

**现象**：`defer` 的注册发生在**每次迭代**（于是有 N 次 `Unlock`），但**执行**发生在函数返回时。在循环进行期间，锁一直被持有：其他 goroutine 全部阻塞在这个锁上；当函数返回时，会连续执行 N 次 `Unlock`——**第 2 次就会 fatal error: sync: unlock of unlocked mutex**（如果中间有其他 goroutine 抢到了锁，则更早在别处炸）。

**✅ 正确写法**：把循环体抽成一个函数，让 `defer` 的作用域跟随单次迭代：

```go
// 片段：省略 package main 与 import，仅示意关键代码（含 ... 占位，不能直接编译）
func good(items []int) {
	for _, it := range items {
		processOne(it) // 锁的获取与释放在这个函数内部配对
	}
}

func processOne(it int) {
	mu.Lock()
	defer mu.Unlock()
	// ... 处理 it
}
```

如果因为性能原因不想抽函数（调用开销、闭包逃逸），就显式解锁，并且**保证每条返回路径都解锁**——这就是 `defer` 存在的意义，通常不值得为它让步。

### 10.9.4 死锁的四个必要条件与 Go 中的典型死锁

**死锁的四个必要条件**（Coffman 条件，全部成立才可能死锁）：

| 条件 | 含义 | 在 Go 中的对应 |
| --- | --- | --- |
| **互斥（Mutual Exclusion）** | 资源同一时刻只能被一个持有者占用 | `Mutex`、容量 1 的 channel、`sync.Map` 的内部锁 |
| **持有并等待（Hold and Wait）** | 持有至少一个资源的同时，等待其他资源 | 拿了锁 A 之后再 `Lock` 锁 B |
| **不可抢占（No Preemption）** | 资源不能被强制夺走，只能由持有者主动释放 | Go 没有"超时解锁"，只能 `Unlock` |
| **循环等待（Circular Wait）** | 存在一个等待的环形链：A 等 B，B 等 A | 两个 goroutine 交叉持锁；或两个 channel 互相等待 |

**破坏任意一个条件就能消除死锁**。工程上最实用的是破坏"循环等待"：**给所有锁定义一个全局顺序，任何地方都按这个顺序加锁**。

下面这个转账示例同时给出了"会死锁的写法"和"正确的写法"。程序本身就是正确版本，死锁版本的输出在注释之后单独给出：

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type account struct {
	id  int // 用一个稳定的 id 作为"加锁顺序"的依据
	mu  sync.Mutex
	bal int
}

// order 按 id 大小返回加锁顺序，保证所有调用者都按同一顺序加锁
func order(a, b *account) (*account, *account) {
	if a.id < b.id {
		return a, b
	}
	return b, a
}

// transfer 正确实现：破坏"循环等待"
func transfer(from, to *account, amount int) bool {
	first, second := order(from, to) // 关键：全局统一的加锁顺序
	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()

	if from.bal < amount {
		return false
	}
	from.bal -= amount
	to.bal += amount
	return true
}

// transferTry 用 TryLock 破坏"持有并等待"：拿不到第二把锁就立刻放弃并释放第一把
func transferTry(from, to *account, amount int) bool {
	first, second := order(from, to)
	if !first.mu.TryLock() {
		return false
	}
	defer first.mu.Unlock()

	if !second.mu.TryLock() {
		return false // 不等了，直接返回；defer 会释放 first
	}
	defer second.mu.Unlock()

	if from.bal < amount {
		return false
	}
	from.bal -= amount
	to.bal += amount
	return true
}

func main() {
	a := &account{id: 1, bal: 1000}
	b := &account{id: 2, bal: 1000}

	var wg sync.WaitGroup
	var okCount atomic.Int64

	// 双向并发转账：如果加锁顺序不统一，这里必然死锁
	for i := 0; i < 500; i++ {
		wg.Go(func() {
			if transfer(a, b, 1) {
				okCount.Add(1)
			}
		})
		wg.Go(func() {
			if transfer(b, a, 1) {
				okCount.Add(1)
			}
		})
	}
	wg.Wait()

	total := a.bal + b.bal
	fmt.Printf("成功转账 %d 次，A=%d B=%d 总额=%d（总额守恒，说明没有丢更新）\n",
		okCount.Load(), a.bal, b.bal, total)

	// TryLock 版本：同样不会死锁，但可能"放弃"，所以总额不一定变化
	if transferTry(a, b, 5) {
		fmt.Println("TryLock 转账成功")
	} else {
		fmt.Println("TryLock 转账放弃（锁被占用）")
	}
}
```

**如果把 `transfer` 里的"统一顺序"去掉，改成各自按参数顺序加锁**，两个 goroutine 分别执行 `transfer(a, b)` 与 `transfer(b, a)` 时就可能出现"A 持有锁 1 等锁 2，B 持有锁 2 等锁 1"，运行时会输出（下面的栈是在 go1.26.6 windows/amd64 上用等价程序实测得到的，**绝对路径与行号随你的代码而变化**）：

```text
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [sync.WaitGroup.Wait]:
sync.runtime_SemacquireWaitGroup(0x33898573a100?, 0x0?)
        D:/software/go/src/runtime/sema.go:114 +0x2e
sync.(*WaitGroup).Wait(0x33898517e080)
        D:/software/go/src/sync/waitgroup.go:206 +0x85
main.main()
        D:/demo/deadlock/main.go:36 +0x165

goroutine 875 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x0?, 0x0?, 0x0?)
        D:/software/go/src/runtime/sema.go:95 +0x25
internal/sync.(*Mutex).lockSlow(0x338985188110)
        D:/software/go/src/internal/sync/mutex.go:149 +0x15d
internal/sync.(*Mutex).Lock(...)
        D:/software/go/src/internal/sync/mutex.go:70
sync.(*Mutex).Lock(...)
        D:/software/go/src/sync/mutex.go:46
main.transfer(0x338985188108, 0x338985188120, 0x1)
        D:/demo/deadlock/main.go:13 +0x53
main.main.func1()
        D:/demo/deadlock/main.go:33 +0x50
created by main.main in goroutine 1
        D:/demo/deadlock/main.go:33 +0x105

goroutine 871 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(0x0?, 0x0?, 0x0?)
        ...
```

注意 Go 1.24 起 `sync.Mutex` 的实现已经搬到 `internal/sync`（`sync.Mutex` 只是一层薄封装），所以栈里会多出 `internal/sync.(*Mutex).lockSlow` 这一帧，而你自己的 `main.transfer` 仍然是定位问题的关键帧。

⚠️ **注意**：这两段输出说明**Go 的死锁检测能力是有限的**。

- 运行时的检测条件是"**所有** goroutine 都睡着了"，此时它会打印 `fatal error: all goroutines are asleep - deadlock!` 并退出（退出码 2）。**这是一次真正的死锁，程序不会永久挂起。**
- 如果还有**任何一个** goroutine 醒着（例如后台有 `time.Ticker` 循环、HTTP 服务器在监听、`sysmon` 之外的用户 goroutine 在跑），运行时**不会**报错，那部分死锁的 goroutine 只是静静地卡着——这就是"部分死锁"，只能靠 goroutine profile 或超时机制发现。
- 栈信息里的 `semacquire` 是"在信号量上等待"的意思，看到它基本就能判定"卡在 Mutex/channel 上"。定位时要找**互相等待**的一组栈：A 的栈里出现 B 持有的锁，B 的栈里出现 A 持有的锁。

### 10.9.5 活锁与饥饿

死锁之外还有两类"看起来在运行，实际上没进展"的问题。

**活锁（Livelock）**：所有 goroutine 都在运行（没有阻塞），但整个系统状态不再前进。典型例子是"两个人都想让路，结果谁都没走过去"。在 Go 里最常见的形态是**无退避的自旋重试**：

```text
❌ 活锁形态（伪代码）
for {
    if !mu.TryLock() {
        continue     // 一直在抢锁，一直在失败，CPU 100% 但毫无进展
    }
    ...
}
```

**症状**：CPU 占用很高，延迟很高，但吞吐极低（成功次数很少）。**修复**：加**退避**（backoff）——随机化重试间隔，或者改用阻塞式 `Lock` 让运行时来管理排队：

```go
// 片段：省略 package main 与 import，仅示意关键代码
for attempt := 0; ; attempt++ {
	if mu.TryLock() {
		break
	}
	// 指数退避 + 随机抖动，避免多个 goroutine 同步重试（惊群）
	time.Sleep(time.Duration(1<<min(attempt, 6)) * time.Millisecond)
}
```

（注意：`min` 是 Go 1.21 引入的内置函数，无需导入。）

**饥饿（Starvation）**：某个 goroutine 长期拿不到资源。在 Go 中的常见成因：

- 读多写少下 `RWMutex` 的写者饥饿 —— 已在 Go 1.9 通过"写者优先"缓解。
- **不公平的 `select`**：如果一个 goroutine 在紧循环里处理高优先级 channel，另一个 channel 可能永远轮不上（`select` 的随机性只是概率上的公平，不保证有界等待）。
- **锁被少数长任务长期持有**：粗粒度临界区 + 大量短任务，短任务永远排在后面（Go 的 `Mutex` 有饥饿模式：等待超过约 1ms 后会切换到 FIFO 的饥饿模式，把锁直接交给队首等待者，从而保证有界等待）。
- **`sync.Pool` 被少数 goroutine 独占**：P 的私有池让"同一个 P 上复用"最快，跨 P 取用要走共享队列，负载不均时会有轻微偏向。

**饥饿与死锁的区别**：饥饿是"**会慢，但会前进**"（有界等待被打破，但最终能拿到），死锁是"**永远不会前进**"。工程上对饥饿的处理手段是：控制临界区长度、限制单个任务的最大耗时、给关键路径加超时。

## 10.10 本章小结

1. **并发是结构，并行是执行。** 并发是把程序拆成可独立执行的部件的能力，单核也能并发；并行是同时执行，必须有多核。先把并发结构做对，再谈并行加速。
2. **goroutine 便宜的两个原因**是"初始栈只有 2 KB 且可按需增长到 1 GB（64 位）"与"用户态 GMP 调度、切换只需约百纳秒量级"。代价是 goroutine 之间没有父子关系，**取消必须显式传播**（`context`）。
3. **通过通信来共享内存**的核心是所有权转移：数据在同一时刻只属于一个 goroutine。但这句话**不是否定 mutex**——`sync` 包是标准库的一等公民，channel 的底层 `hchan` 本身就带锁。选择依据是"数据是流动的还是共享的"。
4. **`main` 返回即进程结束。** 其他 goroutine 被直接杀死，不执行 `defer`。任何"代码写了却没输出"的并发 bug，第一个要怀疑的就是这里。禁止用 `time.Sleep` 代替真正的同步。
5. **`WaitGroup` 四条铁律**：`Add` 必须在 `go` 之前；`defer Done()` 放在 goroutine 第一行；绝不拷贝（传指针，`go vet` 把关）；计数器归零后才复用。Go 1.25 起可以用 `wg.Go(f)` 一步到位。
6. **无缓冲 channel 是同步点（会合），有缓冲 channel 是异步投递（背压）。** `len(ch)` 在并发下不可靠，判断"还有没有数据"必须用 `select` + `default`。
7. **channel 关闭七规则**：只能由发送方关；向已关闭发送 panic；重复关闭 panic；关闭 nil panic；从已关闭接收返回零值且 `ok=false`；关闭会唤醒所有阻塞接收者（广播）；`for range` 依赖关闭才能结束。多个发送方时用"`WaitGroup` + 收尾 goroutine"统一关闭。
8. **nil channel 上是永久阻塞**，它在 `select` 里是"动态禁用分支"的官方技巧，在普通语句里是灾难。
9. **`channel` 是引用类型**是因为它是指向 `hchan`（含环形缓冲区 `buf`、游标 `sendx`/`recvx`、等待队列 `sendq`/`recvq`、一把 `lock`）的指针。性能量级上：atomic 最快，`Mutex` 次之，channel 最慢但语义最强。
10. **`select` 的随机性是实现公平的手段**，不是缺陷；需要优先级必须用嵌套 `select` 显式实现。`default` 实现非阻塞，`time.After` 实现超时，空 `select{}` 永久挂起，`for`+`select` 是事件循环的标准骨架。
11. **同步原语的取舍**：`Mutex` 零值可用但不可重入、必须传指针；`RWMutex` 适合读远多于写，Go 1.9 起写者优先，代价是**递归读锁可能死锁**；`Once` 中 panic 会导致后续调用静默跳过；`sync.Map` 从 Go 1.24 起改由 hash-trie 实现，写与删除不再明显退化，但"读多写少 + 读写不相交的 key"仍是它最擅长的场景；`sync.Pool` 不保证对象存活，归还前必须 `Reset`；`atomic` 提供顺序一致性，但**只保护被原子操作的那一个变量**；`sync.Cond` 必须配锁、必须用 `for` 重查条件，能用 channel 就别用 `Cond`。
12. **数据竞争 = 并发访问同一内存 + 至少一次写 + 无同步。** 它的后果是**未定义行为**，不是"偶尔出错"：可能结果错得离谱、可能崩溃在无关位置、也可能"看起来一直正常"。
13. **`-race` 是必需品但不是万能药。** 它只能发现实际执行到的竞争，有数倍的 CPU/内存开销。CI 里应当把 `go test -race ./...` 作为强制步骤，配合 `go vet ./...` 与 `GORACE=halt_on_error=1`。
14. **并发模式的基础三件套**：生产者-消费者（`close` 是收尾信号）、Worker Pool（固定 worker + 任务 channel + 结果 channel + `WaitGroup` + 收尾关闭）、Fan-in/Fan-out（会打乱顺序）。信号量 = 有缓冲 channel，互斥锁 = 容量 1 的 channel，后者唯一优势是能进 `select`。
15. **死锁的四个必要条件**（互斥、持有并等待、不可抢占、循环等待）破坏任意一个即可；最实用的是"全局统一的加锁顺序"。Go 只会在"所有 goroutine 都睡着"时报告死锁，部分死锁必须靠 pprof 与超时机制发现。**锁粒度过大、循环里 `defer Unlock`、`defer` 在循环中堆积**是最常见的三个锁使用错误。

## 10.11 练习题

### 基础题

**10-1（考察点：`go` 语法与求值时机）**
写一个程序，启动 5 个 goroutine，分别打印 1 到 5。要求：(1) 用**传参**方式传入序号；(2) 用 `sync.WaitGroup` 等待全部结束；(3) 再写一个"错误版本"，把循环变量直接闭包捕获，并说明在 `go.mod` 分别写 `go 1.21` 和 `go 1.26` 时输出行为有何不同。

**10-2（考察点：`main` 退出与同步）**
写一个程序：启动一个 goroutine，它睡 200ms 后打印 "done"。要求分别用 (1) `WaitGroup`、(2) 无缓冲 channel 做完成信号、(3) `chan struct{}` + `close` 广播 三种方式保证输出一定被打印。再写一个用 `time.Sleep(300*time.Millisecond)` 的版本，说明为什么它不可靠。

**10-3（考察点：无缓冲 vs 有缓冲）**
写一个程序，用 `for` 往一个容量为 3 的 channel 里塞 10 个数，同时启动一个消费者不断读取。分别改成无缓冲（容量 0）和容量 10，观察并解释"发送方什么时候会阻塞"，并把 `len(ch)`/`cap(ch)` 的变化打印出来。

**10-4（考察点：channel 关闭规则）**
写一个函数 `merge(chs ...<-chan int) <-chan int` 把多个 channel 合并。要求：(1) 正确关闭输出 channel；(2) 用 `recover` 写一个演示程序，分别触发"向已关闭发送""重复关闭""关闭 nil"三种 panic 并打印错误信息；(3) 说明为什么接收方不能关闭 channel。

### 进阶题

**10-5（考察点：`select` 的随机性与优先级）**
写一个程序：两个 channel 都持续有数据，用 `select` 各消费 10000 次，统计两边被选中的次数并解释结果。然后改造程序，让其中一个 channel 的数据**必须**优先被消费，用嵌套 `select` 实现，并说明"局部优先"与"全局严格优先"的区别。

**10-6（考察点：超时与 goroutine 泄漏）**
写一个 `fetch(url string, timeout time.Duration) (string, error)`，要求：(1) 超时后立即返回错误；(2) **不泄漏**负责请求的 goroutine；(3) 用 `runtime.NumGoroutine()` 在函数被调用 1000 次前后对比，证明没有泄漏。提示：考虑结果 channel 的容量与 `context` 的传播。

**10-7（考察点：`sync.Mutex` 与 `RWMutex` 的选择）**
实现两个版本的"并发安全计数器"：一个用 `sync.Mutex`，一个用 `sync.RWMutex`。在 8 个 goroutine、读写比分别为 100:1、1:1、1:100 的情况下分别计时，用数据说明"读多写少才用 `RWMutex`"，并解释为什么写多时 `RWMutex` 反而更慢。

**10-8（考察点：`sync.Once` 的 panic 陷阱与 `atomic`）**
写一个"只加载一次配置"的函数，要求：(1) 加载失败时**不能**让后续调用静默成功（对比 `once.Do` 中 panic 的行为）；(2) 提供一个 `Reload()` 方法可以强制重新加载，并且 `Reload` 期间其他 goroutine 读到的配置必须是完整的旧值或完整的新值。提示：`atomic.Pointer[T]` + 不可变快照。

### 挑战题

**10-9（考察点：Worker Pool + 优雅退出 + 错误处理）**
实现一个通用 Worker Pool：`Run(ctx context.Context, workers int, jobs <-chan Job, handle func(context.Context, Job) error) error`。要求：(1) `ctx` 取消时所有 worker 都能及时退出，且**不泄漏**；(2) 有一个任务的 `handle` 返回错误时，整个池子停止接收新任务（fail-fast），但要**等待已在执行的任务结束**再返回；(3) 用 `errgroup` 语义手写实现（不允许直接调用 `golang.org/x/sync/errgroup`）；(4) 写测试：提交 100 个任务，其中第 37 个返回错误，验证返回值、验证 `runtime.NumGoroutine()` 回到基线。

**10-10（考察点：死锁排查）**
给定一个"账户转账 + 日志记录"的系统：转账要同时持有两把账户锁，日志写入又要先拿一个全局日志锁。请：(1) 构造出能稳定复现的死锁（提示：让加锁顺序不一致）；(2) 用 `SIGQUIT`（Windows 上按 Ctrl+Break 或使用 `GODEBUG` 相关手段）或 `pprof.Lookup("goroutine")` 抓取栈，指出**互相等待的那一组 goroutine**；(3) 用"全局加锁顺序"改造修复；(4) 说明为什么"部分死锁"（还有其他 goroutine 在跑）不会被运行时检测到。

**10-11（考察点：综合——带并发上限的缓存 + 单飞）**
实现一个并发安全的缓存 `GetOrLoad(key string, load func() (string, error))`，要求：(1) 同一个 key 的并发未命中**只能触发一次** `load`（singleflight 语义），其余调用者共享结果；(2) 有并发上限（例如同时最多 3 个 `load` 在执行），超出时排队而不是直接失败；(3) 支持 TTL 过期与 `Close()` 优雅退出（清理 goroutine 不泄漏）；(4) 在 `-race` 下通过测试，并写一个测试证明"同一个 key 的 `load` 只被调用了一次"。

**10-12（考察点：原理——手写一个基于 channel 的 `WaitGroup`）**
不借助 `sync` 包，只用 channel 实现一个功能等价的 `WaitGroup`（至少支持 `Add`/`Done`/`Wait`），要求：(1) 支持 `Add` 负数；(2) 计数器为负时 panic；(3) `Wait` 在计数器归零后能正确返回；(4) 说明你的实现在哪些边界行为上与标准库不同（例如"计数器归零后再次 `Add` 与 `Wait` 并发"的行为），以及标准库的 `WaitGroup` 内部是如何用 `state` 与信号量做到"零分配"的。


