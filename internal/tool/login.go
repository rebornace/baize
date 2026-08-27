package tool

// LoginRequiredContent is the tool result when invoke is blocked for missing login.
func LoginRequiredContent() map[string]any {
	return map[string]any{"code": "login_required", "message": "此工具需要先登录"}
}
