package domain

// ReviewPermissionMode is the Claude Code permission mode of a review turn when the person switched on
// "Let Claude act without asking during reviews" in Settings. This file is the only place Notted names
// the mode; scripts/check-invariants.sh allows it here and nowhere else.
const ReviewPermissionMode = "bypassPermissions"
