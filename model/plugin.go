package model

import (
	"fmt"
	"reflect"
)

type PluginConfigs map[string]PluginConfig

type PluginConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
	Values  `yaml:",inline"`
}

type Values map[string]interface{}

func (v *Values) UnmarshalYAML(unmarshal func(interface{}) error) error {
	m := map[interface{}]interface{}{}
	if err := unmarshal(&m); err != nil {
		return err
	}
	r, err := ensureMapKeysAreStrings(m)
	if err != nil {
		return err
	}
	*v = r
	return nil
}

func ensureMapKeysAreStrings(m map[interface{}]interface{}) (map[string]interface{}, error) {
	r := map[string]interface{}{}
	for k, v := range m {
		typedKey, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("Unexpected type %s for key %s", reflect.TypeOf(k), k)
		}
		switch v.(type) {
		case map[interface{}]interface{}:
			nestedMapWithStringKeys, err := ensureMapKeysAreStrings(v.(map[interface{}]interface{}))
			if err != nil {
				return nil, err
			}
			r[typedKey] = nestedMapWithStringKeys
		default:
			r[typedKey] = v
		}
	}
	return r, nil
}
