package webhook

import "net/http"

// inboundHandler 的完整实现（验签/幂等/白名单/附件）在任务 6 提供；
// 这里先给存根以保证装配期路由注册可编译。
func (c *Channel) inboundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "webhook inbound not implemented", http.StatusNotImplemented)
	})
}
