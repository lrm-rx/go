package validate

import (
	"fmt"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	"github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zh_translations "github.com/go-playground/validator/v10/translations/zh"
	"reflect"
	"strings"
)

var trans ut.Translator

func init() {
	// 创建翻译器
	uni := ut.New(zh.New())
	trans, _ = uni.GetTranslator("zh")

	// 注册翻译器
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if ok {
		_ = zh_translations.RegisterDefaultTranslations(v, trans)
	}

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		label := field.Tag.Get("label")
		if label == "" {
			label = field.Name
		}
		name := field.Tag.Get("json")
		return fmt.Sprintf("%s---%s", name, label)
	})
}

type ValidateResponse struct {
	FieldMap map[string]any
	Msg      string
}

func ValidateError(err error) (res ValidateResponse) {
	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		res.Msg = err.Error()
		return
	}
	var m = map[string]any{}
	var msgList []string
	for _, e := range errs {
		msg := e.Translate(trans)
		_list := strings.Split(msg, "---")
		m[_list[0]] = _list[1]
		msgList = append(msgList, _list[1])
	}
	res.FieldMap = m
	res.Msg = strings.Join(msgList, ";")
	return
}
