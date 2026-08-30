package connector

import (
	"github.com/rebornace/baize/internal/store"
)

// MergeOpts configures MergeCatalog.
type MergeOpts struct {
	Existing        []store.Tool
	Discovered      []store.Tool
	RequireLogin    *[]string
	RequireApproval []string
}

// MergeCatalog merges the persisted connector tool catalog with the freshly
// discovered spec/plugin rows. Extra rows are always preserved; spec/plugin
// rows are kept only when re-discovered, carrying over Enabled, Export, Title
// (and RequireLogin when RequireLogin is nil) from the prior row.
func MergeCatalog(opts MergeOpts) []store.Tool {
	existingByName := map[string]store.Tool{}
	for _, t := range opts.Existing {
		existingByName[t.Name] = t
	}

	discoveredSet := map[string]bool{}
	for _, t := range opts.Discovered {
		discoveredSet[t.Name] = true
	}

	loginNil := opts.RequireLogin == nil
	approvalEmpty := len(opts.RequireApproval) == 0

	loginSet := map[string]bool{}
	if !loginNil {
		for _, n := range *opts.RequireLogin {
			loginSet[n] = true
		}
	}
	approvalSet := map[string]bool{}
	for _, n := range opts.RequireApproval {
		approvalSet[n] = true
	}

	var out []store.Tool

	// Extra rows are always preserved verbatim.
	for _, t := range opts.Existing {
		if t.Source == store.ToolSourceExtra {
			out = append(out, t)
		}
	}

	// Discovered spec/plugin rows replace their prior counterparts.
	for _, d := range opts.Discovered {
		row := d
		if ex, ok := existingByName[d.Name]; ok && (ex.Source == store.ToolSourceSpec || ex.Source == store.ToolSourcePlugin || ex.Source == store.ToolSourceMCP) {
			row = ex
			row.Method = d.Method
			row.Path = d.Path
			if !ex.DescriptionCustom {
				row.Description = d.Description
			}
			row.Source = d.Source
			row.OperationID = d.OperationID
			if !loginNil {
				row.RequireLogin = false
			}
		} else {
			row.Enabled = true
			row.RequireLogin = false
		}
		out = append(out, row)
	}

	// Final rewrite pass for explicit login/approval lists.
	if !loginNil || !approvalEmpty {
		for i, r := range out {
			if r.Source == store.ToolSourceExtra {
				if !loginNil {
					out[i].RequireLogin = loginSet[r.Name]
				}
				continue
			}
			if !discoveredSet[r.Name] {
				continue
			}
			if !loginNil {
				out[i].RequireLogin = loginSet[r.Name]
			}
			if !approvalEmpty {
				out[i].RequireApproval = approvalSet[r.Name]
			}
		}
	}

	return out
}
