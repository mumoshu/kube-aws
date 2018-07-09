package pluginutil

import (
	"fmt"

	"github.com/kubernetes-incubator/kube-aws/plugin/pluginmodel"
	"reflect"
)

func MergeValues(v pluginmodel.Values, o map[string]interface{}) pluginmodel.Values {
	r := merge(map[string]interface{}(v), map[string]interface{}(o))
	switch r := r.(type) {
	case map[string]interface{}:
		return pluginmodel.Values(r)
	}
	panic(fmt.Errorf("error in type assertion to map[string]interface{} from merge result: %v", r))
}

func merge(x1, x2 interface{}) interface{} {
	switch x1 := x1.(type) {
	case map[string]interface{}:
		switch x2 := x2.(type) {
		case map[string]interface{}:
			for k, v2 := range x2 {
				if v1, ok := x1[k]; ok {
					x1[k] = merge(v1, v2)
				} else {
					x1[k] = v2
				}
			}
			return x1
		default:
			panic(fmt.Sprintf("cannot merge %+v(map[string]interface{}) and %+v(%s)", x1, x2, reflect.TypeOf(x2)))
		}
	case map[string]string:
		switch x2 := x2.(type) {
		case map[string]string:
			for k, v2 := range x2 {
				x1[k] = v2
			}
			r := map[string]interface{}{}
			for k, v := range x1 {
				r[k] = string(v)
			}
			return r

		default:
			panic(fmt.Sprintf("cannot merge %+v(map[string]string map[string]string) and %+v(%s)", x1, x2, reflect.TypeOf(x2)))
		}
	case nil:
		panic(fmt.Sprintf("cannot merge nil and %+v", x2))
	}
	return x2
}
