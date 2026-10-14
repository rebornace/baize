package loginmanage

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const managedKindConnectorLogin = "connector_login"

type skillFrontmatter struct {
	Name               string   `yaml:"name"`
	Description        string   `yaml:"description"`
	Tools              []string `yaml:"tools"`
	Managed            bool     `yaml:"managed"`
	ManagedKind        string   `yaml:"managed_kind"`
	ManagedConnectorID string   `yaml:"managed_connector_id"`
}

// RenderSKILLMD builds managed login skill markdown for connectorID and tools.
func RenderSKILLMD(connectorID string, tools []string) (string, error) {
	skillID := SkillID(connectorID)
	if skillID == "" {
		return "", fmt.Errorf("empty skill id for connector %q", connectorID)
	}
	fm := skillFrontmatter{
		Name:               skillID,
		Description:        fmt.Sprintf("连接器 %s 的登录相关能力（由连接器自动维护）", connectorID),
		Tools:              append([]string(nil), tools...),
		Managed:            true,
		ManagedKind:        managedKindConnectorLogin,
		ManagedConnectorID: connectorID,
	}
	yamlBytes, err := yaml.Marshal(&fm)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(yamlBytes)
	if !strings.HasSuffix(string(yamlBytes), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("---\n\n")
	b.WriteString("# ")
	b.WriteString(skillID)
	b.WriteString("\n\n")
	b.WriteString("本技能由连接器自动维护。可通过 `@" + skillID + "` 或设置中激活。\n\n")
	b.WriteString("## 优先：浏览器托管登录（推荐）\n\n")
	b.WriteString("若白泽已为该连接器启用「对外登录 / 托管登录页」，**不要**在对话里向用户索要账号、密码、Token 或验证码。\n\n")
	b.WriteString("请明确告诉用户优先点输入框上方的 **「浏览器登录」** 横幅，或失败工具卡片上的同名按钮；若界面没有这些入口，再引导：设置 → 对外登录 → 打开登录页。登录完成后回来继续。\n\n")
	b.WriteString("仅当用户明确要求「在对话里登录」、或托管登录不可用时，才使用下方登录工具，并在缺必填参数时询问用户。\n\n")
	b.WriteString("用户激活本技能（包括只发送 `@" + skillID + "`）即表示要登录。本技能已在本轮激活，不要再调用 `activate_skill`。优先引导浏览器登录；不要只问候或空转。\n\n")
	b.WriteString("## 可用工具\n\n")
	if len(tools) == 0 {
		b.WriteString("（当前无登录相关启用工具）\n\n")
	} else {
		for _, name := range tools {
			b.WriteString("- `")
			b.WriteString(name)
			b.WriteString("`\n")
		}
		b.WriteByte('\n')
	}
	b.WriteString("登录可能需要多步（发码、校验、OAuth 回调信息等）；请根据工具返回结果继续下一步。\n\n")
	b.WriteString("登录工具成功后，检查结果里的 `session_captured`：为 true 才表示凭证已写入当前会话，后续业务工具会自动带上；为 false 或缺失时不要说「会话已保存」，应说明未捕获到 token，并请用户检查登录返回或连接器 capture 配置。\n\n")
	b.WriteString("最佳实践：不要把密码写进与登录无关的用户气泡；有托管登录时禁止在对话中收集密码。\n")
	return b.String(), nil
}
