package global

import "sync/atomic"

var liveAccountSuperCode atomic.Value

// SetLiveAccountSuperCode 发布不可变快照，配置热更新不与注册请求竞争。
func SetLiveAccountSuperCode(code string) { liveAccountSuperCode.Store(code) }

func LiveAccountSuperCode() string {
	code, _ := liveAccountSuperCode.Load().(string)
	return code
}
