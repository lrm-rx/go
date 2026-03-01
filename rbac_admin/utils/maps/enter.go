package maps

import "reflect"

func StructToMaps(data any, tag string) (mps map[string]any) {
	mps = make(map[string]any)
	v := reflect.ValueOf(data)
	t := reflect.TypeOf(data)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		column := t.Field(i).Tag.Get(tag)
		if column == "-" || column == "" {
			continue
		}
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				continue
			}
			mps[column] = field.Elem().Interface()
			continue
		}
		mps[column] = field.Interface()
	}
	return
}
