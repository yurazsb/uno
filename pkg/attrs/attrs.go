package attrs

import "sync"

// Attrs 是属性存储的接口，支持基本的操作如 Set、Get、Has、Del 等
type Attrs[K comparable, V any] interface {
	Set(K, V)                    // 设置属性
	Del(K)                       // 删除属性
	Get(K) (V, bool)             // 获取属性，返回值与存在标志
	Has(K) bool                  // 判断属性是否存在
	Keys() []K                   // 获取所有属性的键
	Values() []V                 // 获取所有属性的值
	Len() int                    // 获取属性的数量
	Clear()                      // 清空属性
	Clone(deep bool) Attrs[K, V] // 克隆属性存储
	Merge(other Attrs[K, V])     // 合并另一个属性存储
	Range(func(K, V) bool)       // 遍历所有属性
	ToMap() map[K]V              // 转换为 map
}

// New 构造函数，选择是否启用线程安全
func New[K comparable, V any](mutex bool) Attrs[K, V] {
	if mutex {
		return &mutexAttrs[K, V]{data: make(map[K]V)}
	}
	return &plainAttrs[K, V]{data: make(map[K]V)}
}

// plainAttrs 是非线程安全的属性存储实现
type plainAttrs[K comparable, V any] struct {
	data map[K]V
}

// mutexAttrs 是线程安全的属性存储实现
type mutexAttrs[K comparable, V any] struct {
	data map[K]V
	lock sync.RWMutex
}

// Set 设置属性
func (a *plainAttrs[K, V]) Set(key K, value V) {
	a.data[key] = value
}

// Del 删除属性
func (a *plainAttrs[K, V]) Del(key K) {
	delete(a.data, key)
}

// Get 获取属性
func (a *plainAttrs[K, V]) Get(key K) (V, bool) {
	val, ok := a.data[key]
	return val, ok
}

// Has 判断属性是否存在
func (a *plainAttrs[K, V]) Has(key K) bool {
	_, ok := a.data[key]
	return ok
}

// Keys 获取所有属性的键
func (a *plainAttrs[K, V]) Keys() []K {
	keys := make([]K, 0, len(a.data))
	for k := range a.data {
		keys = append(keys, k)
	}
	return keys
}

// Values 获取所有属性的值
func (a *plainAttrs[K, V]) Values() []V {
	values := make([]V, 0, len(a.data))
	for _, v := range a.data {
		values = append(values, v)
	}
	return values
}

// Len 获取属性数量
func (a *plainAttrs[K, V]) Len() int {
	return len(a.data)
}

// Clear 清空属性存储
func (a *plainAttrs[K, V]) Clear() {
	a.data = make(map[K]V)
}

// Clone 克隆属性存储（浅拷贝）
func (a *plainAttrs[K, V]) Clone(deep bool) Attrs[K, V] {
	newMap := make(map[K]V, len(a.data))
	for k, v := range a.data {
		newMap[k] = v
	}
	return &plainAttrs[K, V]{data: newMap}
}

// Merge 合并另一个属性存储
func (a *plainAttrs[K, V]) Merge(other Attrs[K, V]) {
	other.Range(func(k K, v V) bool {
		a.data[k] = v
		return true
	})
}

// Range 遍历所有属性
func (a *plainAttrs[K, V]) Range(f func(K, V) bool) {
	for k, v := range a.data {
		if !f(k, v) {
			break
		}
	}
}

// ToMap 转换为 map
func (a *plainAttrs[K, V]) ToMap() map[K]V {
	m := make(map[K]V, len(a.data))
	for k, v := range a.data {
		m[k] = v
	}
	return m
}

// Set 设置属性（线程安全）
func (a *mutexAttrs[K, V]) Set(key K, value V) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.data[key] = value
}

// Del 删除属性（线程安全）
func (a *mutexAttrs[K, V]) Del(key K) {
	a.lock.Lock()
	defer a.lock.Unlock()
	delete(a.data, key)
}

// Get 获取属性（线程安全）
func (a *mutexAttrs[K, V]) Get(key K) (V, bool) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	val, ok := a.data[key]
	return val, ok
}

// Has 判断属性是否存在（线程安全）
func (a *mutexAttrs[K, V]) Has(key K) bool {
	a.lock.RLock()
	defer a.lock.RUnlock()
	_, ok := a.data[key]
	return ok
}

// Keys 获取所有属性的键（线程安全）
func (a *mutexAttrs[K, V]) Keys() []K {
	a.lock.RLock()
	defer a.lock.RUnlock()
	keys := make([]K, 0, len(a.data))
	for k := range a.data {
		keys = append(keys, k)
	}
	return keys
}

// Values 获取所有属性的值（线程安全）
func (a *mutexAttrs[K, V]) Values() []V {
	a.lock.RLock()
	defer a.lock.RUnlock()
	values := make([]V, 0, len(a.data))
	for _, v := range a.data {
		values = append(values, v)
	}
	return values
}

// Len 获取属性数量（线程安全）
func (a *mutexAttrs[K, V]) Len() int {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return len(a.data)
}

// Clear 清空属性存储（线程安全）
func (a *mutexAttrs[K, V]) Clear() {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.data = make(map[K]V)
}

// Clone 克隆属性存储（浅拷贝，线程安全）
func (a *mutexAttrs[K, V]) Clone(deep bool) Attrs[K, V] {
	a.lock.RLock()
	defer a.lock.RUnlock()
	newMap := make(map[K]V, len(a.data))
	for k, v := range a.data {
		newMap[k] = v
	}
	return &mutexAttrs[K, V]{data: newMap}
}

// Merge 合并另一个属性存储（线程安全）
func (a *mutexAttrs[K, V]) Merge(other Attrs[K, V]) {
	a.lock.Lock()
	defer a.lock.Unlock()
	other.Range(func(k K, v V) bool {
		a.data[k] = v
		return true
	})
}

// Range 遍历所有属性（线程安全）
func (a *mutexAttrs[K, V]) Range(f func(K, V) bool) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	for k, v := range a.data {
		if !f(k, v) {
			break
		}
	}
}

// ToMap 转换为 map（线程安全）
func (a *mutexAttrs[K, V]) ToMap() map[K]V {
	a.lock.RLock()
	defer a.lock.RUnlock()
	m := make(map[K]V, len(a.data))
	for k, v := range a.data {
		m[k] = v
	}
	return m
}
