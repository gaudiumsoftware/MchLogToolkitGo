package mchlogcommon

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/rs/zerolog"
)

// GetJSONLogger retorna o evento de log correspondente
// pronto para ser persistido no arquivo.
func GetJSONLogger(logger *zerolog.Logger, content any) (*zerolog.Event, error) {
	var err error
	var event *zerolog.Event
	var ok bool

	v := reflect.ValueOf(content)
	switch v.Kind() {
	case reflect.Map:
		var m map[string]any
		if m, ok = content.(map[string]any); !ok {
			m = make(map[string]any)
			keys := v.MapKeys()
			for _, k := range keys {
				va := v.MapIndex(k)
				switch va.Kind() {
				case reflect.String:
					m[k.String()] = va.String()
				case reflect.Int, reflect.Int64, reflect.Int32:
					m[k.String()] = va.Int()
				case reflect.Float64, reflect.Float32:
					m[k.String()] = va.Float()
				}
			}
		}
		event = logger.Log().Fields(m)

	case reflect.Slice, reflect.Array:
		var arrb []byte
		if arrb, ok = content.([]byte); ok {
			var m map[string]any
			if err = json.Unmarshal(arrb, &m); err == nil {
				event = logger.Log().Fields(m)
			}
		} else {
			var arr []any
			if arr, ok = content.([]any); !ok {
				tam := v.Len()
				for i := 0; i < tam; i++ {
					va := v.Index(i)
					switch va.Kind() {
					case reflect.String:
						arr = append(arr, va.String())
					case reflect.Int, reflect.Int64, reflect.Int32:
						arr = append(arr, va.Int())
					case reflect.Float64, reflect.Float32:
						arr = append(arr, va.Float())
					}
				}
			}
			event = logger.Log().Fields(arr)
		}
	case reflect.String:
		s := content.(string)
		var m map[string]any
		if err = json.Unmarshal([]byte(s), &m); err == nil {
			event = logger.Log().Fields(m)
		}
	default:
		err = errors.New("")
	}

	if err != nil {
		err = fmt.Errorf("tipo inválido do conteúdo do log: %v.\nEsperados map, json em formato de string ou []byte", v.Type())
	}

	return event, err
}
