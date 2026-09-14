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
	b.WriteString("登录成功后，凭证仅保留在当前会话中。\n\n")
	b.WriteString("最佳实践：不要把密码写进与登录无关的用户气泡。\n")
	return b.String(), nil
}
