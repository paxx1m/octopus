package op

import (
	"encoding/json"
	"reflect"
	"sync"

	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm/schema"
)

// partialUpdate 收集非空指针字段，生成 GORM 部分更新所需的 map。
// 消除 ChannelUpdate / GroupUpdate 中手写「if req.X != nil { fields = append(...) }」样板。
//
// 注意：
//   - 必须用 newPartialUpdate() 构造；零值（nil map）上调用 set 会 panic。
//   - GORM 的 Updates 仅接受字面 map[string]interface{}（命名类型会报 unsupported data），
//     提交前用 gormMap() 转换。
//   - map 更新不经过 GORM serializer，serializer:json 字段（base_urls / custom_header）
//     由 set 手动编码为 JSON 字符串，与 struct 更新结果一致。
type partialUpdate map[string]any

var (
	partialSchemaOnce sync.Once
	partialSchemas    = map[reflect.Type]*schema.Schema{}
)

func partialSchemaFor(dest any) *schema.Schema {
	t := reflect.TypeOf(dest)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	partialSchemaOnce.Do(func() {
		for _, d := range []any{&model.Channel{}, &model.ChannelKey{}, &model.Group{}} {
			if s, err := schema.Parse(d, &sync.Map{}, schema.NamingStrategy{}); err == nil {
				partialSchemas[reflect.TypeOf(d).Elem()] = s
			}
		}
	})
	return partialSchemas[t]
}

func newPartialUpdate() partialUpdate {
	return make(partialUpdate)
}

// set 仅当 ptr 为非 nil 指针时记录该字段；值为 *T 时按 T 写入（零值也写入，与显式传值语义一致）。
// dest 用于解析 serializer 字段（仅用于 GORM map 更新路径的编码补偿）。
func (p partialUpdate) set(dest any, column string, ptr any) {
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	value := rv.Elem().Interface()
	if s := partialSchemaFor(dest); s != nil {
		if f, ok := s.FieldsByDBName[column]; ok {
			if _, isSerializer := f.TagSettings["SERIALIZER"]; isSerializer {
				if b, err := json.Marshal(value); err == nil {
					p[column] = string(b)
					return
				}
			}
		}
	}
	p[column] = value
}

// gormMap 返回字面 map[string]interface{} 类型（GORM 类型断言要求）。
func (p partialUpdate) gormMap() map[string]interface{} {
	return p
}
