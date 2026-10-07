// Package shader 提供着色器模块的接口反射、兼容性判定与运行期热重载能力。
package shader

// Stage 表示着色器阶段。
type Stage string

const (
	StageVertex   Stage = "vertex"
	StageFragment Stage = "fragment"
	StageCompute  Stage = "compute"
)

// DataType 表示接口中变量/成员的类型。
type DataType string

const (
	TypeFloat     DataType = "float"
	TypeVec2      DataType = "vec2"
	TypeVec3      DataType = "vec3"
	TypeVec4      DataType = "vec4"
	TypeMat4      DataType = "mat4"
	TypeInt       DataType = "int"
	TypeSampler2D DataType = "sampler2D"
	TypeTexture2D DataType = "texture2D"
)

// IOVariable 描述一个阶段输入或输出变量。
type IOVariable struct {
	Name     string
	Type     DataType
	Location int
}

// ConstantField 描述常量缓冲中的一个字段。
type ConstantField struct {
	Name   string
	Type   DataType
	Offset int // 字节偏移
	Size   int // 字节大小
}

// ConstantBuffer 描述一个常量缓冲（uniform block）。
type ConstantBuffer struct {
	Name    string
	Binding int
	Size    int // 总字节大小
	Fields  []ConstantField
}

// ResourceBinding 描述一个资源绑定（纹理、采样器等）。
type ResourceBinding struct {
	Name    string
	Type    DataType
	Binding int
	Stages  []Stage // 需要该资源的阶段
}

// ShaderInterface 是一次反射的完整结果，描述着色器对外接口。
// 反射结果按绑定位置/名称排序，保证可重复。
type ShaderInterface struct {
	Stages         []Stage
	Inputs         []IOVariable
	Outputs        []IOVariable
	ConstantBuffer []ConstantBuffer
	Resources      []ResourceBinding
}
