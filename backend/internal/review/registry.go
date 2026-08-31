package review

import (
	"context"
	"fmt"
)

// GetScheduler 按名称返回调度器。
func GetScheduler(name string) (ReviewScheduler, error) {
	switch name {
	case "", "simple_v1":
		return NewSimpleV1(), nil
	default:
		return nil, fmt.Errorf("unknown review scheduler: %s", name)
	}
}

var _ = context.Background
