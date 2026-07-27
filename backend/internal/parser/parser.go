package parser

import (
	"drone-sim-backend/internal/trajectory"
	"io"
)

// RouteParser 航线解析器接口
type RouteParser interface {
	Parse(r io.Reader, filename string) (*trajectory.Trajectory, error)
	SupportedExtensions() []string
}

// ParserRegistry 解析器注册表
type ParserRegistry struct {
	parsers map[string]RouteParser
}

// NewRegistry 创建新的解析器注册表
func NewRegistry() *ParserRegistry {
	return &ParserRegistry{
		parsers: make(map[string]RouteParser),
	}
}

// Register 注册解析器，将其支持的所有扩展名映射到该解析器
func (r *ParserRegistry) Register(parser RouteParser) {
	for _, ext := range parser.SupportedExtensions() {
		r.parsers[ext] = parser
	}
}

// FindParser 根据文件扩展名查找对应的解析器
func (r *ParserRegistry) FindParser(ext string) (RouteParser, bool) {
	p, ok := r.parsers[ext]
	return p, ok
}
