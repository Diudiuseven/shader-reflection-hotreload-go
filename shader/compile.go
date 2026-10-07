package shader

// CompiledModule 是编译产物，内嵌反射信息，保证反射结果与实际生效绑定一致。
type CompiledModule struct {
	name   string
	iface  *Interface
	binary []byte
}

// Name 返回模块名。
func (m *CompiledModule) Name() string { return m.name }

// Compiler 把着色器源编译为产物。
type Compiler interface {
	Compile(name, source string) (*CompiledModule, error)
}

// FakeCompiler 是确定性的进程内编译器实现，用于测试与离线管线。
type FakeCompiler struct{}

// NewFakeCompiler 构造一个 FakeCompiler。
func NewFakeCompiler() *FakeCompiler { return &FakeCompiler{} }

// Compile 解析源文本并生成产物；源不合法（如被截断）时返回 *CompileError。
func (c *FakeCompiler) Compile(name, source string) (*CompiledModule, error) {
	iface, err := parseSource(source)
	if err != nil {
		if ce, ok := err.(*CompileError); ok {
			ce.Source = name
		}
		return nil, err
	}
	return &CompiledModule{name: name, iface: iface, binary: []byte(source)}, nil
}
