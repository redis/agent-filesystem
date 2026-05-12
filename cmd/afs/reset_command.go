package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func cmdReset() error {
	targetStatePath := statePath()
	if st, err := loadStateFromPath(targetStatePath); err == nil {
		if err := stopRuntimeForReset(st, targetStatePath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	removedConfig := false
	if err := os.Remove(configPath()); err == nil {
		removedConfig = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	removedState, err := removeResetScopedState(targetStatePath)
	if err != nil {
		return err
	}

	rows := []outputRow{
		{Label: "config", Value: ternaryString(removedConfig, compactDisplayPath(configPath()), "already clear")},
		{Label: "state", Value: ternaryString(removedState, filepath.Base(stateDir()), "already clear")},
		{Label: "next", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" setup")},
	}
	printSection(markerSuccess+" "+clr(ansiBold, "local state reset"), rows)
	return nil
}

func stopRuntimeForReset(st state, targetStatePath string) error {
	reg, err := loadMountRegistry()
	if err != nil {
		return err
	}
	if localPath := st.LocalPath; localPath != "" {
		if rec, ok := removeMountByPath(&reg, localPath); ok {
			return unmountMountRecord(reg, rec, false)
		}
	}
	if handled, err := stopSyncServicesIfActiveAtPath(st, targetStatePath, false); handled || err != nil {
		return err
	}
	return nil
}

func stopSyncServicesIfActiveAtPath(st state, targetStatePath string, deleteLocal bool) (bool, error) {
	if strings.TrimSpace(st.Mode) != modeSync {
		return false, nil
	}

	fmt.Println()

	if st.SyncPID > 0 && processAlive(st.SyncPID) {
		s := startStep("Stopping sync daemon")
		if err := terminatePID(st.SyncPID, 5*time.Second); err != nil {
			s.fail(err.Error())
		} else {
			s.succeed(fmt.Sprintf("pid %d", st.SyncPID))
		}
	}
	if localPath := strings.TrimSpace(st.LocalPath); localPath != "" {
		if st.ReadOnly {
			if err := releaseReadonlyLocalTree(localPath); err != nil {
				fmt.Printf("  %s local sync folder remains read-only at %s (%v)\n", clr(ansiYellow, "!"), localPath, err)
			}
		}
		if deleteLocal {
			if err := os.RemoveAll(localPath); err != nil {
				fmt.Printf("  %s local sync folder preserved at %s (%v)\n", clr(ansiYellow, "!"), localPath, err)
			}
		}
	}

	if deleteLocal {
		// Clean up sync state only when the user explicitly deletes the local
		// copy; otherwise it remains as the baseline for a later re-mount.
		workspace := strings.TrimSpace(st.CurrentWorkspace)
		_ = removeSyncState(workspace)
	}
	closeManagedWorkspaceSession(configFromState(st), strings.TrimSpace(st.CurrentWorkspace), strings.TrimSpace(st.SessionID))

	if err := os.Remove(targetStatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	local := "preserved"
	if deleteLocal {
		local = "deleted"
	}
	fmt.Printf("Unmounted workspace %s\n", currentWorkspaceLabel(st.CurrentWorkspace))
	fmt.Printf("path   %s\n", homeRelativeDisplayPath(st.LocalPath))
	fmt.Printf("local  %s\n", local)
	return true, nil
}

func removeResetScopedState(targetStatePath string) (bool, error) {
	removed := false
	if err := os.Remove(targetStatePath); err == nil {
		removed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	reg, err := loadMountRegistry()
	if err != nil {
		return removed, err
	}
	if len(reg.Mounts) == 0 {
		if err := os.Remove(mountRegistryPath()); err == nil {
			removed = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err := os.Remove(legacyMountRegistryPath()); err == nil {
			removed = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}

	for _, dir := range []string{filepath.Dir(targetStatePath), syncStateDir(), stateDir()} {
		if err := removeDirIfEmpty(dir); err != nil {
			return false, err
		}
	}
	if _, err := os.Stat(stateDir()); errors.Is(err, os.ErrNotExist) {
		removed = true
	}
	return removed, nil
}

func removeDirIfEmpty(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if errors.Is(err, os.ErrExist) || errors.Is(err, syscall.ENOTEMPTY) {
		return nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Err == syscall.ENOTEMPTY {
		return nil
	}
	return err
}
