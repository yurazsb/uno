# Changelog

## v0.1.0 - 2025-08-23
### Added
- 初始发布：TCP/UDP 统一抽象（Serve/Dial + Hook）
- Framer / Codec / Middleware 基础能力
- Conn 线程模型：read/write/control + 协程池
- 示例：Echo Server / Echo Client

### Notes
- 零第三方依赖，纯标准库实现
- API 暂定为预览阶段（v0.x）
