package redis

import "github.com/bytedance/sonic"

func marshalJSON(v interface{}) ([]byte, error)      { return sonic.Marshal(v) }
func unmarshalJSON(data []byte, v interface{}) error { return sonic.Unmarshal(data, v) }
