package procfs

// Human-side skill-draft telemetry (PROC-FS A4). Appended to the source run only.
const (
	EventHumanSkillDraftOpened    = "human.skill_draft.opened"
	EventHumanSkillDraftPolished  = "human.skill_draft.polished"
	EventHumanSkillDraftInstalled = "human.skill_draft.installed"
)

const maxHITLCommentRunes = 80
