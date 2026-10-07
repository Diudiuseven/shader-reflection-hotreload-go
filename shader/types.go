// Package shader 提供着色器模块的接口反射、兼容性判定与运行期热重载能力。
package shader

// Stage 表示着色器阶段。
type Stage int

const (
	StageVertex Stage = iota
	StageFragment
	StageCompute
)

func (s Stage) String() string {
	switch s {
	case StageVertex:
		return "vertex"
	case StageFragment:
		return "fragment"
	case StageCompute:
		return "compute"
	}
	return "unknown"
}

// Stages 是阶段位掩码集合。
type Stages uint8

const (
	StageVertexBit   Stages = 1 << StageVertex
	StageFragmentBit Stages = 1 << StageFragment
	StageComputeBit  Stages = 1 << StageCompute
)

// Has 报告掩码中是否包含指定阶段。
func (m Stages) Has(s Stage) bool { return m&(1<<s) != 0 }

// DataType 描述接口成员的数据类型。
type DataType string

const (
	TypeFloat  DataType = "float"
	TypeFloat2 DataType = "float2"
	TypeFloat3 DataType = "float3"
	TypeFloat4 DataType = "float4"
	TypeInt    DataType = "int"
	TypeUint   DataType = "uint"
	TypeMat4   DataType = "mat4"
)

// ResourceKind 描述资源绑定的类别。
type ResourceKind string

const (
	ResourceTexture2D     ResourceKind = "texture2d"
	ResourceSampler       ResourceKind = "sampler"
	ResourceStorageBuffer ResourceKind = "storage_buffer"
)

// IOVariable 描述一个阶段输入/输出变量。
type IOVariable struct {
	Name     string
	Type     DataType
	Location int
}

// ConstantBuffer 描述一个常量缓冲绑定。
type ConstantBuffer struct {
	Name    string
	Binding int
	Size    int // 字节数
	Stages  Stages
}

// ResourceBinding 描述一个资源绑定（纹理、采样器、存储缓冲等）。
type ResourceBinding struct {
	Name    string
	Kind    ResourceKind
	Binding int
	Stages  Stages
}

// Interface 是一次反射得到的着色器对外接口。
// 反射结果必须与实际生效的绑定一致且可重复（字段顺序稳定）。
type Interface struct {
	Stages          []Stage
	Inputs          []IOVariable
	Outputs         []IOVariable
	ConstantBuffers []ConstantBuffer
	Resources       []ResourceBinding
}
