package services

import (
	"fmt"
	"sync/atomic"
	"time"
)

var idSeq atomic.Uint64

// newID 生成唯一 ID：时间戳加进程内递增序号。
// 以前只用 UnixNano，时钟精度为微秒的平台上连续创建会得到相同 ID，后创建的对象覆盖前一个
func newID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), idSeq.Add(1))
}
