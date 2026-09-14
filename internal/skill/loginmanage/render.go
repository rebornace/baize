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
	b.WriteString("用户激活本技能（包括只发送 `@" + skillID + "`、没有其它说明）即表示要登录。本技能已在本轮激活，不要再调用 `activate_skill`。请立刻使用下方列出的登录相关工具开始登录；缺必填参数时向用户询问，不要只问候或空转。\n\n")
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
	b.WriteString("最佳实践：不要把密码写进与登录无关的用户气泡。\n")
	return b.String(), nil
}
