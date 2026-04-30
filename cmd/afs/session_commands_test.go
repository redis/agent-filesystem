package main

import (
	"testing"

	"github.com/redis/agent-filesystem/internal/controlplane"
)

func TestChangelogDisplayOpUsesHumanLabels(t *testing.T) {
	tests := []struct {
		name string
		row  controlplane.ChangelogEntryRow
		want string
	}{
		{
			name: "new file put is create",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpPut},
			want: "Create",
		},
		{
			name: "existing file put is update",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpPut, PrevHash: "blob-old"},
			want: "Update",
		},
		{
			name: "mkdir is create folder",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpMkdir},
			want: "Create folder",
		},
		{
			name: "rmdir is delete folder",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpRmdir},
			want: "Delete folder",
		},
		{
			name: "new symlink is create link",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpSymlink},
			want: "Create link",
		},
		{
			name: "existing symlink is update link",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpSymlink, PrevHash: "target-old"},
			want: "Update link",
		},
		{
			name: "chmod is change mode",
			row:  controlplane.ChangelogEntryRow{Op: controlplane.ChangeOpChmod},
			want: "Change mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changelogDisplayOp(tt.row); got != tt.want {
				t.Fatalf("changelogDisplayOp() = %q, want %q", got, tt.want)
			}
		})
	}
}
