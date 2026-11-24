package dto

// AppVersionResponse is returned to a client before it enters the application.
type AppVersionResponse struct {
	Platform          string             `json:"platform"`
	LatestVersionCode int                `json:"latest_version_code"`
	LatestVersionName string             `json:"latest_version_name"`
	Updates           []AppVersionUpdate `json:"updates"`
}

type AppVersionUpdate struct {
	VersionCode         int                  `json:"version_code"`
	VersionName         string               `json:"version_name"`
	ReleaseNotes        []string             `json:"release_notes"` // 供尚未升级的 App 使用。
	ReleaseNotesGrouped *ReleaseNotesGrouped `json:"release_notes_grouped"`
	UncategorizedNotes  []string             `json:"uncategorized_notes,omitempty"`
}

// ReleaseNotesGrouped preserves the author-specified categories.
type ReleaseNotesGrouped struct {
	Features     []string `json:"features"`
	Improvements []string `json:"improvements"`
	BugFixes     []string `json:"bug_fixes"`
	Notices      []string `json:"notices"`
}
