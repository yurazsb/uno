package handler

import (
	"container/list"
	"github.com/yurazsb/uno/pkg/trie"
	"strings"
	"sync"
	"sync/atomic"
)

// RouterHandler 返回一个路由分发 Handler。
// resolver 是一个函数，用于从 core.Context 中提取请求路径（或路由标识），
// 并返回该路径及一个布尔值表示是否有效。
// 分发逻辑：
// 1. 如果 pathResolver 返回 false，则直接调用 next()。
// 2. 否则查找 Router 中匹配的 Route。
// 3. 如果匹配成功，则执行匹配 Route 的 middleware 链。
// 4. 如果未匹配，则执行 Router 的 NotFound 处理链。
// 5. 最终调用 next()。
var RouterHandler = func(resolver func(ctx Context) (string, bool), router *Router) Handler {
	return func(ctx Context, next func()) {
		chain := NewChain()

		path, ok := resolver(ctx)
		if !ok {
			next()
			return
		}

		route, matched := router.Match(path)
		if matched {
			chain.Use(route.Handlers()...)
		} else {
			load := router.notFound.Load()
			if load != nil {
				chain.Use(*load...)
			}
		}

		chain.Use(func(_ Context, _ func()) { next() })
		chain.Handler(ctx)
	}
}

// Router 是一个路由器，支持分组、前缀、middleware 链及路径匹配。
// 内部使用 Trie 存储路由路径，实现高性能查询。
type Router struct {
	*RouterGroup                           // 根分组
	sep          string                    // 路径分隔符
	trie         *trie.Trie                // 路径前缀树
	cache        atomic.Pointer[lruCache]  // LRU 缓存
	notFound     atomic.Pointer[[]Handler] // 未匹配路由时的处理链
}

// NewRouter 创建一个新的 Router，指定路径分隔符。
var NewRouter = func(sep string) *Router {
	router := &Router{
		sep:  sep,
		trie: trie.New(),
	}
	router.RouterGroup = &RouterGroup{router: router, middleware: make([]Handler, 0)}
	return router
}

// SplitPath 将路径按 Router 的 sep 分割为字符串切片，并去除空白部分。
func (r *Router) SplitPath(path string) []string {
	path = strings.Trim(path, r.sep)
	if path == "" {
		return nil
	}
	parts := strings.Split(path, r.sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// JoinPath 将路径片段按 sep 拼接成完整路径，并去除空白部分。
func (r *Router) JoinPath(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, r.sep)
}

// Lru 设置 LRU 缓存大小
func (r *Router) Lru(size int) {
	if size <= 0 {
		r.cache.Store(nil)
	}
	r.cache.Store(newLRUCache(size))
}

// NotFound 设置 Router 的未匹配路由处理链。
func (r *Router) NotFound(handlers ...Handler) {
	if len(handlers) == 0 {
		return
	}
	r.notFound.Store(&handlers)
}

// Match 根据路径查询路由。
// 返回匹配的 Match 及匹配结果。
func (r *Router) Match(path string) (*Route, bool) {
	// 1. 先查缓存
	cache := r.cache.Load()
	if cache != nil {
		if val, ok := cache.Get(path); ok {
			return val, true
		}
	}

	// 2. 查 Trie
	parts := r.SplitPath(path)
	value, ok := r.trie.Query(parts...)
	if !ok || value == nil {
		return nil, false
	}
	route, ok := value.(*Route)
	if !ok {
		return nil, false
	}

	// 3. 更新缓存
	cache = r.cache.Load()
	if cache != nil {
		cache.Add(path, route)
	}
	return route, true
}

// 注册路由
func (r *Router) insert(route *Route) {
	path := route.Path()
	parts := r.SplitPath(path)
	r.trie.Insert(route, parts...)

	// 新路由加入时清空缓存（避免旧缓存冲突）
	cache := r.cache.Load()
	if cache != nil {
		cache.Clear()
	}
}

// RouterGroup 支持分组路由，包含前缀、middleware 链及父子关系。
type RouterGroup struct {
	prefix   string
	router   *Router
	parent   *RouterGroup
	children []*RouterGroup // 子分组列表

	mu         sync.RWMutex
	middleware []Handler

	cacheValid     atomic.Bool
	cachedHandlers []Handler
}

// Group 创建一个子分组，继承父分组的 Router。
func (g *RouterGroup) Group(prefix string) *RouterGroup {
	if prefix == "" {
		return g
	}
	child := &RouterGroup{
		router: g.router,
		parent: g,
		prefix: g.router.JoinPath(g.prefix, prefix),
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.children = append(g.children, child)
	return child
}

// Use 为当前 RouterGroup 添加 middleware。
// middleware 会从外向内执行。
func (g *RouterGroup) Use(handlers ...Handler) {
	if len(handlers) == 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.middleware = append(g.middleware, handlers...)
	// 删除缓存
	g.invalidateCache()
}

// Handle 为当前分组注册路径及其处理链。
func (g *RouterGroup) Handle(path string, handlers ...Handler) {
	if len(handlers) == 0 {
		return
	}
	fullPath := g.router.JoinPath(g.prefix, path)
	r := &Route{group: g, path: fullPath, middleware: handlers}
	g.router.insert(r)
}

// Handlers 返回当前分组及其父分组的 middleware 链，按外层到内层顺序组合。
func (g *RouterGroup) Handlers() []Handler {
	// 先读缓存
	if g.cacheValid.Load() {
		return append([]Handler(nil), g.cachedHandlers...)
	}

	// 计算 handlers
	var combined []Handler
	for current := g; current != nil; current = current.parent {
		current.mu.RLock()
		combined = append(current.middleware, combined...)
		current.mu.RUnlock()
	}

	// 写缓存
	if g.cacheValid.CompareAndSwap(false, true) {
		g.cachedHandlers = combined
	}
	return combined
}

// invalidateCache 递归失效缓存
func (g *RouterGroup) invalidateCache() {
	g.cacheValid.Store(false)
	g.mu.RLock()
	children := g.children
	g.mu.RUnlock()
	for _, child := range children {
		child.invalidateCache()
	}
}

// Route 表示单条路由，包含其所在分组、路径及 middleware 链。
type Route struct {
	group      *RouterGroup
	path       string
	middleware []Handler
}

// Path 返回 Route 的注册路径。
func (r *Route) Path() string {
	return r.path
}

// Handlers 返回 Route 的完整处理链，包括分组 middleware 与自身 middleware。
func (r *Route) Handlers() []Handler {
	return append(r.group.Handlers(), r.middleware...)
}

// --- LRU 缓存实现 ---

type lruCache struct {
	mu       sync.Mutex
	capacity int
	cache    map[string]*list.Element
	ll       *list.List
}

type entry struct {
	key   string
	value *Route
}

func newLRUCache(capacity int) *lruCache {
	return &lruCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		ll:       list.New(),
	}
}

func (c *lruCache) Get(key string) (*Route, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		return ele.Value.(*entry).value, true
	}
	return nil, false
}

func (c *lruCache) Add(key string, value *Route) {
	if c.capacity == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		ele.Value.(*entry).value = value
		return
	}
	ele := c.ll.PushFront(&entry{key, value})
	c.cache[key] = ele
	if c.ll.Len() > c.capacity {
		c.removeOldest()
	}
}

func (c *lruCache) removeOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
	}
}

func (c *lruCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*list.Element)
	c.ll.Init()
}
