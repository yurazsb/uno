package trie

import (
	"sync/atomic"
)

// Trie 前缀树
type Trie struct {
	root *Node
}

// New 构造一个前缀树
func New() *Trie {
	return &Trie{root: &Node{}}
}

// Query 查询节点，无锁安全
func (t *Trie) Query(parts ...string) (any, bool) {
	cur := t.root
	for _, part := range parts {
		child, ok := cur.Child(part)
		if !ok {
			return nil, false
		}
		cur = child
	}

	v := cur.Value()
	return v, v != nil
}

// Insert 插入节点，线程安全，Copy-On-Write
func (t *Trie) Insert(value any, parts ...string) {
	cur := t.root
	for _, part := range parts {
		for {
			children := cur.Children()
			if children == nil {
				// 初始化空 map
				newMap := make(map[string]*Node)
				if cur.children.CompareAndSwap(nil, &newMap) {
					children = newMap
				} else {
					continue
				}
			}

			child, ok := children[part]
			if !ok {
				// 创建新节点
				newChild := &Node{part: part}
				// Copy-On-Write
				newMap := make(map[string]*Node, len(children)+1)
				for k, v := range children {
					newMap[k] = v
				}
				newMap[part] = newChild
				if cur.children.CompareAndSwap(&children, &newMap) {
					child = newChild
				} else {
					continue // CAS 失败重试
				}
			}
			cur = child
			break
		}
	}
	cur.value.Store(value)
}

// Node 节点结构
type Node struct {
	part     string
	value    atomic.Value                     // 原子存储节点值
	children atomic.Pointer[map[string]*Node] // 原子存储子节点 map
}

// Part 返回节点 part
func (n *Node) Part() interface{} {
	return n.part
}

// Value 返回节点值
func (n *Node) Value() any {
	return n.value.Load()
}

// Child 查询子节点
func (n *Node) Child(part string) (*Node, bool) {
	children := n.Children()
	if children == nil {
		return nil, false
	}
	child, ok := children[part]
	return child, ok
}

// Children 获取子节点 map，如果为空返回 nil
func (n *Node) Children() map[string]*Node {
	ptr := n.children.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}
