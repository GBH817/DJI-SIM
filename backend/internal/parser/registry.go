package parser

var defaultRegistry = NewRegistry()

// DefaultRegistry 返回全局默认解析器注册表
func DefaultRegistry() *ParserRegistry {
	return defaultRegistry
}
