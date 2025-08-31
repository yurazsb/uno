# Changelog

## v0.1.3 - 2025-08-23
### Added
- 初始发布：TCP/UDP 统一抽象（Serve/Dial + Hook）
- Framer / Codec / Middleware 基础能力
- Conn 线程模型：read/write/control + 协程池
- 示例：Echo Server / Echo Client
### Notes
- 零第三方依赖，纯标准库实现
- API 暂定为预览阶段（v0.x）


## v0.1.4 - 2025-08-31
### Changed
- **Send API**：方法签名由 `Send(msg any) error` 改为 `Send(msg any) <-chan error`，  
  现在返回一个异步错误通道，用于更灵活地处理发送结果。⚠️ 此为 **Breaking Change**，请更新调用方式。
- 文档优化：改进了核心特性介绍，调整部分术语表达，使整体更清晰易懂。