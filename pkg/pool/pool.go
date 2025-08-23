package pool

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

/********** 配置与监控 **********/

type PanicHandler func(any)

type Options struct {
	MaxWorkers   int           // 并发上限（必填，>0）
	Queue        int           // 队列容量（>=0；0 表示无队列，直接触发扩容或丢弃/阻塞）
	IdleTimeout  time.Duration // 工作者空闲回收超时
	NonBlocking  bool          // true: 队列满则立即返回 false；false: 可阻塞
	EnqueueWait  time.Duration // 非阻塞失败时最多等待此时长（0=不等待）
	PanicHandler PanicHandler  // panic 捕获
}

type Option func(*Options)

func WithMaxWorkers(n int) Option            { return func(o *Options) { o.MaxWorkers = n } }
func WithQueue(n int) Option                 { return func(o *Options) { o.Queue = n } }
func WithIdleTimeout(d time.Duration) Option { return func(o *Options) { o.IdleTimeout = d } }
func WithNonBlocking() Option                { return func(o *Options) { o.NonBlocking = true } }
func WithEnqueueWait(d time.Duration) Option { return func(o *Options) { o.EnqueueWait = d } }
func WithPanicHandler(h PanicHandler) Option { return func(o *Options) { o.PanicHandler = h } }

/********** WorkerPool 实现 **********/

type Stats struct {
	Workers   int
	QueueLen  int
	Submitted uint64
	Dropped   uint64
}

type Pool interface {
	Submit(task func()) bool
	SubmitCtx(ctx context.Context, task func()) error
	TrySubmit(task func()) bool
	Resize(maxWorkers int)
	Close()
	Stats() Stats
}

type workerPool struct {
	opts Options

	tasks  chan func()
	stopCh chan struct{}
	wg     sync.WaitGroup

	curWorkers int32
	submitted  uint64
	dropped    uint64
	closed     atomic.Bool
}

func New(opts ...Option) Pool {
	o := Options{
		MaxWorkers:  runtime.GOMAXPROCS(0) * 4,
		Queue:       1024,
		IdleTimeout: 30 * time.Second,
		NonBlocking: true,
	}
	for _, fn := range opts {
		fn(&o)
	}
	if o.MaxWorkers <= 0 {
		o.MaxWorkers = 1
	}
	if o.Queue < 0 {
		o.Queue = 0
	}

	p := &workerPool{
		opts:   o,
		tasks:  make(chan func(), o.Queue),
		stopCh: make(chan struct{}),
	}
	return p
}

func (p *workerPool) Stats() Stats {
	return Stats{
		Workers:   int(atomic.LoadInt32(&p.curWorkers)),
		QueueLen:  len(p.tasks),
		Submitted: atomic.LoadUint64(&p.submitted),
		Dropped:   atomic.LoadUint64(&p.dropped),
	}
}

func (p *workerPool) TrySubmit(task func()) bool {
	return p.submitInternal(task, false, 0)
}

func (p *workerPool) Submit(task func()) bool {
	// 非阻塞模式：仍可能返回 false
	// 阻塞模式：会在队列有空位前阻塞
	return p.submitInternal(task, !p.opts.NonBlocking, 0)
}

func (p *workerPool) SubmitCtx(ctx context.Context, task func()) error {
	if p.closed.Load() {
		return errors.New("pool closed")
	}
	// 尝试扩容
	p.maybeSpawnWorker()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.tasks <- p.wrap(task):
		atomic.AddUint64(&p.submitted, 1)
		return nil
	default:
		// 如果允许等待一段时间
		if p.opts.EnqueueWait > 0 {
			timer := time.NewTimer(p.opts.EnqueueWait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				atomic.AddUint64(&p.dropped, 1)
				return errors.New("enqueue timeout")
			case p.tasks <- p.wrap(task):
				atomic.AddUint64(&p.submitted, 1)
				return nil
			}
		}
		// 不等
		if !p.opts.NonBlocking {
			// 阻塞直到可入队或关闭
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-p.stopCh:
				return errors.New("pool closed")
			case p.tasks <- p.wrap(task):
				atomic.AddUint64(&p.submitted, 1)
				return nil
			}
		}
		atomic.AddUint64(&p.dropped, 1)
		return errors.New("queue full")
	}
}

func (p *workerPool) submitInternal(task func(), block bool, wait time.Duration) bool {
	if p.closed.Load() {
		return false
	}
	p.maybeSpawnWorker()

	// 快速路径：尝试入队
	select {
	case p.tasks <- p.wrap(task):
		atomic.AddUint64(&p.submitted, 1)
		return true
	default:
	}

	// 入队失败
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
			atomic.AddUint64(&p.dropped, 1)
			return false
		case p.tasks <- p.wrap(task):
			atomic.AddUint64(&p.submitted, 1)
			return true
		}
	}

	if block && !p.opts.NonBlocking {
		// 阻塞直到可入队或关闭
		select {
		case <-p.stopCh:
			return false
		case p.tasks <- p.wrap(task):
			atomic.AddUint64(&p.submitted, 1)
			return true
		}
	}

	// 放弃
	atomic.AddUint64(&p.dropped, 1)
	return false
}

func (p *workerPool) maybeSpawnWorker() {
	// 如果队列有压力且 worker 未达上限，尝试扩容
	for {
		cw := atomic.LoadInt32(&p.curWorkers)
		if int(cw) >= p.opts.MaxWorkers {
			return
		}
		// 仅在任务排队或无 worker 时启动新 worker
		if len(p.tasks) == 0 && cw > 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&p.curWorkers, cw, cw+1) {
			p.spawn()
			return
		}
	}
}

func (p *workerPool) spawn() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer atomic.AddInt32(&p.curWorkers, -1)

		idle := time.NewTimer(p.opts.IdleTimeout)
		defer idle.Stop()

		for {
			select {
			case <-p.stopCh:
				return
			case task := <-p.tasks:
				if !idle.Stop() {
					<-idle.C
				}
				// 执行
				task()
				idle.Reset(p.opts.IdleTimeout)
			case <-idle.C:
				// 超时回收
				return
			}
		}
	}()
}

func (p *workerPool) wrap(task func()) func() {
	if p.opts.PanicHandler == nil {
		return task
	}
	return func() {
		defer func() {
			if r := recover(); r != nil {
				p.opts.PanicHandler(r)
			}
		}()
		task()
	}
}

func (p *workerPool) Resize(maxWorkers int) {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	p.opts.MaxWorkers = maxWorkers
}

func (p *workerPool) Close() {
	if p.closed.CompareAndSwap(false, true) {
		close(p.stopCh)
		p.wg.Wait()
		// 不关闭 tasks chan，避免生产者 panic；由 GC 回收
	}
}

/********** OrderedExecutor（单线程有序执行器） **********/

type OrderedExecutor interface {
	Submit(task func()) bool // FIFO、同线程
	SubmitCtx(ctx context.Context, task func()) error
	Close()
	Len() int
}

type orderedExecutor struct {
	q      chan func()
	once   sync.Once
	closed atomic.Bool
	pool   Pool   // 底层用哪个池执行“串行调度器”
	name   string // 便于调试
}

type OrderedOption func(*orderedExecutor)

func WithExecutorBuffer(n int) OrderedOption {
	return func(e *orderedExecutor) { e.q = make(chan func(), n) }
}
func WithExecutorName(name string) OrderedOption {
	return func(e *orderedExecutor) { e.name = name }
}

func NewOrderedExecutor(p Pool, opts ...OrderedOption) OrderedExecutor {
	e := &orderedExecutor{
		q:    make(chan func(), 1024),
		pool: p,
		name: "ordered",
	}
	for _, fn := range opts {
		fn(e)
	}
	return e
}

func (e *orderedExecutor) start() {
	e.once.Do(func() {
		// 用底层 Pool 跑一个“永生 worker”，串行消费队列
		_ = e.pool.SubmitCtx(context.Background(), func() {
			for task := range e.q {
				// 同线程顺序执行
				task()
			}
		})
	})
}

func (e *orderedExecutor) Submit(task func()) bool {
	if e.closed.Load() {
		return false
	}
	e.start()
	select {
	case e.q <- task:
		return true
	default:
		// 队列满则丢弃（也可改成阻塞/等待）
		return false
	}
}

func (e *orderedExecutor) SubmitCtx(ctx context.Context, task func()) error {
	if e.closed.Load() {
		return errors.New("executor closed")
	}
	e.start()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e.q <- task:
		return nil
	}
}

func (e *orderedExecutor) Close() {
	if e.closed.CompareAndSwap(false, true) {
		close(e.q)
	}
}

func (e *orderedExecutor) Len() int { return len(e.q) }
